package confermadoc

// L1 — le regole pure della conferma del documento resa funzione del core (direttiva 35, C-3; direttiva 39, S8 passo 2).

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func TestV1S8FileStep(t *testing.T) {
	for _, c := range []struct {
		est, nome string
		atteso    bool
	}{
		{"stp", "x.stp", true}, {".STEP", "x", true}, {"", "pezzo.Step", true}, {"pdf", "x.stp", false}, {"", "senza", false},
	} {
		if got := FileStep(c.est, c.nome); got != c.atteso {
			t.Errorf("FileStep(%q, %q) = %v, atteso %v", c.est, c.nome, got, c.atteso)
		}
	}
}

func TestV1S8LaSceltaDellaRevisione(t *testing.T) {
	c := db.Componente{ComponenteID: uuid.New(), Codice: "7120001"}
	d := db.Documento{DocumentoID: uuid.New(), NomeFile: "a.stp", Tipo: db.TipoDocumentoCad3d}
	// niente da chiedere: la scelta cade
	if s, err := VerificaScelta("b.stp", c, db.TipoDocumentoCad3d, nil, Scelta{Data: true}); err != nil || s.Data {
		t.Fatalf("senza correnti: %+v %v", s, err)
	}
	// un corrente senza risposta: rifiuto del Fascicolo (lo riconoscono le pagine legacy)
	_, err := VerificaScelta("b.stp", c, db.TipoDocumentoCad3d, []db.Documento{d}, Scelta{})
	var r fascicolo.Rifiuto
	if !errors.As(err, &r) || !strings.Contains(string(r), "«aggiungi»") {
		t.Fatalf("un corrente senza risposta: %v", err)
	}
	// «aggiungi» (la risposta di S8: non sostituisce mai)
	if s, err := VerificaScelta("b.stp", c, db.TipoDocumentoCad3d, []db.Documento{d}, Scelta{Data: true}); err != nil || !s.Data || s.Sostituisce != uuid.Nil {
		t.Fatalf("aggiungi: %+v %v", s, err)
	}
	// sostituire lo STEP strutturale vuole la risposta sul riferimento
	c.StepStrutturaleID = uuid.NullUUID{UUID: d.DocumentoID, Valid: true}
	if _, err := VerificaScelta("b.stp", c, db.TipoDocumentoCad3d, []db.Documento{d}, Scelta{Data: true, Sostituisce: d.DocumentoID}); err == nil {
		t.Fatal("la sostituzione dello STEP strutturale senza risposta passa")
	}
}

func TestV1S8IlCodiceDalComponente(t *testing.T) {
	c := db.Componente{Codice: "7120001A"}
	if got, err := CodiceDaComponente("7120001a", c, false); err != nil || got != "7120001A" {
		t.Fatalf("stesso codice con altre maiuscole: %q %v", got, err)
	}
	if _, err := CodiceDaComponente("999", c, false); err == nil {
		t.Fatal("un codice diverso senza correggi passa")
	}
	if got, err := CodiceDaComponente("999", c, true); err != nil || got != "7120001A" {
		t.Fatalf("correggi: %q %v", got, err)
	}
}

func TestV1S8LeRegoleDelCliente(t *testing.T) {
	if !RegolaBool(json.RawMessage(`{}`), "cartella_per_codice", true) || RegolaBool(json.RawMessage(`{"cartella_per_codice":false}`), "cartella_per_codice", true) {
		t.Fatal("RegolaBool")
	}
	if ptxt("  ") != (pgtype.Text{}) || tronca(strings.Repeat("é", 5), 3) != "ééé" {
		t.Fatal("ptxt o tronca")
	}
}
