package web

import (
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// TestLaFraseDellaRianalisiDiceIPdf (Smistamento F9, analizzatore 3 → 4): il gesto «Rianalizza» accoda anche i
// PDF letti da un analizzatore precedente, e la frase dice di quali file parla, contati a parte dagli STEP.
// Correzione F9: quando non c'e' niente da fare la frase nomina STEP e PDF (prima diceva «Nessuno STEP da
// rileggere», che dal F9 descriveva meno di quello che il gesto ha guardato).
//
// K4: anche i DXF analizzati senza il testo, contati a parte dai PDF. La frase di «niente da fare» resta quella.
//
// LS-01: anche gli STEP riaccodati perché la loro struttura è in una forma che il server non accetta (in pratica la v4 di
// un worker di GP-3), contati a parte dalle analisi mancanti, dopo di loro e prima dei PDF. Correzioni dei verificatori:
// la frase non dice perché (lo stesso numero conta anche una v2, una versione oltre la 4, una struttura assente o che non
// si decodifica, e «letta da un worker non aggiornato» sarebbe falso per qualcuno; il log del server dice il perché, con
// il worker, solo per una struttura che si decodifica, non per una assente, che non si decodifica o v1); e per uno STEP
// da rileggere non più in staging dice «Riscarica, poi di nuovo Rianalizza», perché «Riscarica» da solo riapplica i
// fatti presenti.
// Lo STEP riletto con i fatti correnti è la frase che il gesto dava su uno STEP v4 prima di LS-01, e che dà dopo la
// rilettura (le frasi del gesto vero sono in ls01_rianalizza_db_test.go).
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
		{fascicolo.Rianalisi{PdfAccodati: 1, DxfAccodati: 2, DxfGiaInCoda: 1, DxfRimandati: 1, DxfSenzaStaging: 1},
			"1 PDF accodato per rileggerne il testo, 2 DXF accodati per rileggerne il testo, 1 DXF già in coda, " +
				"1 DXF rimandato al prossimo giro, 1 DXF non più in staging (Riscarica)."},
		{fascicolo.Rianalisi{Riletti: 2, StepRiaccodati: 1, StepGiaInCoda: 1, PdfAccodati: 1},
			"2 STEP riletti con le regole del cliente, 1 STEP accodato per rileggerne la struttura, " +
				"1 STEP già in coda per rileggerne la struttura, 1 PDF accodato per rileggerne il testo."},
		{fascicolo.Rianalisi{Accodati: 1, StepRiaccodati: 3, StepRimandati: 2, StepSenzaStaging: 1},
			"1 analisi accodata, 3 STEP accodati per rileggerne la struttura, " +
				"2 STEP da rileggere rimandati al prossimo giro, 1 STEP da rileggere non più in staging (Riscarica, poi di nuovo Rianalizza)."},
		{fascicolo.Rianalisi{Riletti: 1}, "1 STEP riletto con le regole del cliente."},
		{fascicolo.Rianalisi{}, "Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente."},
	}
	for _, c := range casi {
		if got := fraseRianalisi(c.ri); got != c.frase {
			t.Errorf("%+v: %q, attesa %q", c.ri, got, c.frase)
		}
	}
}
