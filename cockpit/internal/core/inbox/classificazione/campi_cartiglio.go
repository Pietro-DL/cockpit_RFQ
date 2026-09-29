package classificazione

import (
	"encoding/json"
	"strings"

	"promatec/cockpit/internal/platform/contratti/worker"
)

// I campi del cartiglio che nominano il pezzo, com'e' scritto (Smistamento, giro 4, fase 4.17a: il censimento
// delle forme). Il censimento non cerca codici: conta come sono fatti i valori che il cliente scrive nel campo
// «codice» o «numero di disegno», anche quando nessuna famiglia li riconosce, perche' e' guardandoli che una
// famiglia si scrive. Le letture (LettureDelPDF) passano dal Motore e darebbero solo cio' che le regole di oggi
// gia' sanno.
//
// La regola di F9 resta: i fatti del worker si leggono solo in questo package (testoDelPDF, con il controllo
// della versione di DecodificaTestoPDF). Di qui esce un campo gia' interpretato: una delle due etichette, il
// valore ripulito, la fonte con i nomi delle letture.

// CampoIdentificativo e' il valore di un campo del probabile cartiglio che nomina il pezzo.
type CampoIdentificativo struct {
	Etichetta string // worker.CampoCodice o worker.CampoNumeroDisegno
	Valore    string // com'e' scritto, senza spazi in testa e in coda
	Fonte     string // FonteTestoCartiglio, FonteTestoPagina, FonteOCR: le fonti delle letture (EvidenzeTestoPDF)
}

// Indizio dice se il campo e' stato letto con l'OCR: come per le letture, un indizio (F9).
func (c CampoIdentificativo) Indizio() bool { return c.Fonte == FonteOCR }

// CampiIdentificativiDelPDF sono i campi codice e numero di disegno del probabile cartiglio nei fatti di
// un'analisi, nell'ordine del worker. Pura. Niente se il testo non c'e' o non si legge (fatti di prima
// dell'analizzatore 4, versione sconosciuta, PDF che non si apre: testoDelPDF), niente per un campo vuoto o di
// una fonte che la classificazione non conosce. La fonte segue EvidenzeTestoPDF: il testo nativo nella zona in
// basso a destra e' «testo_cartiglio», fuori zona «testo_pagina», l'OCR «ocr» in qualunque zona.
func CampiIdentificativiDelPDF(fatti json.RawMessage) []CampoIdentificativo {
	t, _ := testoDelPDF(fatti)
	if t == nil {
		return nil
	}
	var out []CampoIdentificativo
	for _, c := range t.Cartiglio {
		if c.Etichetta != worker.CampoCodice && c.Etichetta != worker.CampoNumeroDisegno {
			continue
		}
		v := strings.TrimSpace(c.Valore)
		if v == "" {
			continue
		}
		var fonte string
		switch {
		case c.Fonte == worker.FonteTestoOCR:
			fonte = FonteOCR
		case c.Fonte == worker.FonteTestoNativo && c.Zona == worker.ZonaBassoDestra:
			fonte = FonteTestoCartiglio
		case c.Fonte == worker.FonteTestoNativo:
			fonte = FonteTestoPagina
		default:
			continue
		}
		out = append(out, CampoIdentificativo{Etichetta: c.Etichetta, Valore: v, Fonte: fonte})
	}
	return out
}
