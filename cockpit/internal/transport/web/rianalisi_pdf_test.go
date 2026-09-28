package web

import (
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// TestLaFraseDellaRianalisiDiceIPdf (Smistamento F9, analizzatore 3 → 4): il gesto «Rianalizza» accoda anche i
// PDF letti da un analizzatore precedente, e la frase dice di quali file parla, contati a parte dagli STEP.
// Correzione F9: quando non c'e' niente da fare la frase nomina STEP e PDF (prima diceva «Nessuno STEP da
// rileggere», che dal F9 descriveva meno di quello che il gesto ha guardato).
func TestLaFraseDellaRianalisiDiceIPdf(t *testing.T) {
	casi := []struct {
		ri    fascicolo.Rianalisi
		frase string
	}{
		{fascicolo.Rianalisi{Riletti: 1, Accodati: 1, PdfAccodati: 2, PdfGiaInCoda: 1},
			"1 STEP riletto con le regole del cliente, 1 analisi accodata, 2 PDF accodati per rileggerne il testo, 1 PDF già in coda."},
		{fascicolo.Rianalisi{PdfAccodati: 1, PdfRimandati: 3, PdfSenzaStaging: 1},
			"1 PDF accodato per rileggerne il testo, 3 PDF rimandati al prossimo giro, 1 PDF non più in staging (Riscarica)."},
		{fascicolo.Rianalisi{Rimandati: 1, SenzaStaging: 2}, "1 STEP rimandato al prossimo giro, 2 STEP non più in staging (Riscarica)."},
		{fascicolo.Rianalisi{}, "Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente."},
	}
	for _, c := range casi {
		if got := fraseRianalisi(c.ri); got != c.frase {
			t.Errorf("%+v: %q, attesa %q", c.ri, got, c.frase)
		}
	}
}
