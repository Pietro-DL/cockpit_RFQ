package classificazione

import (
	"encoding/json"
	"reflect"
	"testing"
)

// L1 — Smistamento, giro 4, fase 4.17a (il censimento delle forme): i campi del cartiglio che nominano il pezzo
// escono dalla classificazione gia' interpretati. Solo codice e numero di disegno, il valore senza spazi, la
// fonte con i nomi delle letture (nativo in basso a destra, nativo altrove, OCR in qualunque zona); niente per
// un campo vuoto, per una fonte sconosciuta, per fatti senza testo_pdf o di una versione che non si conosce.
func TestICampiIdentificativiDelPDF(t *testing.T) {
	fatti := json.RawMessage(`{"testo_pdf": {"versione": 1, "estraibile": true, "cartiglio": [
		{"etichetta": "codice", "valore": " 7120001A ", "fonte": "nativo", "zona": "basso_destra", "pagina": 1},
		{"etichetta": "numero_disegno", "valore": "7120001", "fonte": "ocr", "zona": "basso_destra", "pagina": 1},
		{"etichetta": "numero_disegno", "valore": "7120010", "fonte": "nativo", "zona": "pagina", "pagina": 1},
		{"etichetta": "revisione", "valore": "1", "fonte": "nativo", "zona": "basso_destra", "pagina": 1},
		{"etichetta": "titolo", "valore": "SUPPORTO", "fonte": "nativo", "zona": "basso_destra", "pagina": 1},
		{"etichetta": "codice", "valore": "  ", "fonte": "nativo", "zona": "basso_destra", "pagina": 1},
		{"etichetta": "codice", "valore": "7120011", "fonte": "stampa", "zona": "basso_destra", "pagina": 1}]}}`)
	got := CampiIdentificativiDelPDF(fatti)
	atteso := []CampoIdentificativo{
		{Etichetta: "codice", Valore: "7120001A", Fonte: FonteTestoCartiglio},
		{Etichetta: "numero_disegno", Valore: "7120001", Fonte: FonteOCR},
		{Etichetta: "numero_disegno", Valore: "7120010", Fonte: FonteTestoPagina},
	}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("campi: %+v", got)
	}
	if got[0].Indizio() || !got[1].Indizio() {
		t.Error("l'OCR e' un indizio, il testo nativo no")
	}
	// un PDF senza testo nativo (curve, scansione) porta i campi dell'OCR: il testo c'e', ed e' un indizio
	senza := json.RawMessage(`{"testo_pdf": {"versione": 1, "estraibile": false, "cartiglio": [
		{"etichetta": "codice", "valore": "7120001", "fonte": "ocr", "zona": "basso_destra", "pagina": 1}]}}`)
	if c := CampiIdentificativiDelPDF(senza); len(c) != 1 || c[0].Fonte != FonteOCR {
		t.Errorf("il PDF senza testo nativo: %+v", c)
	}
	for nome, f := range map[string]string{
		"versione sconosciuta":   `{"testo_pdf": {"versione": 0, "cartiglio": [{"etichetta": "codice", "valore": "7120001", "fonte": "nativo"}]}}`,
		"senza testo_pdf":        `{"struttura": {"versione": 2}}`,
		"il solo elenco":         `[{"etichetta": "codice", "valore": "7120001", "fonte": "nativo"}]`,
		"il PDF che non si apre": `{"errore_pdf": "cifrato"}`,
		"vuoto":                  ``,
	} {
		if c := CampiIdentificativiDelPDF(json.RawMessage(f)); c != nil {
			t.Errorf("%s: nessun campo, ne ha dati %+v", nome, c)
		}
	}
}
