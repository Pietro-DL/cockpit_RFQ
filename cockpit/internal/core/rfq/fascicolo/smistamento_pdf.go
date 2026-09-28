package fascicolo

// Il punto d'aggancio fra il testo dei PDF (Smistamento F9) e il flusso ancorato al prodotto (F8).
//
// La separazione e' quella degli STEP: il worker estrae FATTI dal contenuto (analisi_fatti), il core li
// normalizza in EVIDENZE, e il flusso consuma solo evidenze normalizzate. Il flusso non legge mai il JSON del
// worker: ne' quello degli STEP (usa le righe di componente_proposta e relazione_proposta che ApplicaStruttura
// ne ha tratto) ne' quello dei PDF, di cui qui non si conosce e non si inventa il formato. La lettura
// strutturata dei PDF (parole e blocchi con le coordinate, i candidati del cartiglio, l'OCR selettivo) la fa
// l'altra linea; quando c'e', la sua normalizzazione riempie EvidenzeContenutoPDF, e il flusso le usa senza
// cambiare: un codice letto nel contenuto di un PDF e' un'evidenza indipendente dal nome del file, come la
// radice di uno STEP (decisioni dell'utente del 27/09 ter).
//
// Oggi EvidenzeContenutoPDF non restituisce niente, e il flusso tratta ogni PDF come un PDF senza testo: il
// codice del file viene solo dal suo nome, e un PDF non ancora mai niente (prova
// TestSenzaEvidenzeIlPdfSiComportaComeUnPdfSenzaTesto, in smistamento_test.go).
// L'integrazione — l'ancora «piatta» dal PDF del prodotto (A-P1, A-P2), le prove 163 e 164, le discordanze
// con il cartiglio — si fa dopo l'unione delle due linee.

import (
	"context"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// Le fonti di un'evidenza dal contenuto di un PDF: dove, nel file, e' stato letto il codice.
const (
	FontePDFCartiglio = "testo_cartiglio" // testo nativo, fra i candidati del cartiglio
	FontePDFTesto     = "testo"           // testo nativo, altrove nella pagina
	FontePDFMetadati  = "metadati"        // titolo o soggetto del documento
	FontePDFOCR       = "ocr"             // letto con l'OCR: un'evidenza, non una decisione
)

// EvidenzaContenutoPDF e' un codice letto DENTRO un PDF, gia' normalizzato dal core: la forma neutra che il
// flusso consuma, qualunque sia il formato dei fatti del worker da cui viene. Il codice e' quello che il
// Motore delle regole del cliente della RFQ riconosce; la pagina e la posizione dicono dove, per mostrarlo.
type EvidenzaContenutoPDF struct {
	Fonte     string       `json:"fonte"` // FontePDF*
	Codice    string       `json:"codice"`
	Rev       string       `json:"rev,omitempty"`
	Pagina    int          `json:"pagina,omitempty"` // da 1; 0 = non si sa (i metadati)
	Posizione *RiquadroPDF `json:"posizione,omitempty"`
	Famiglia  string       `json:"famiglia,omitempty"` // la famiglia del cliente che ha riconosciuto il codice
}

// RiquadroPDF e' la posizione di un'evidenza nella pagina, in punti, con l'origine in alto a sinistra.
type RiquadroPDF struct {
	X0 float64 `json:"x0"`
	Y0 float64 `json:"y0"`
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
}

// DalCartiglio dice se l'evidenza e' del cartiglio (testo nativo o OCR della sua zona): la lettura che il
// flusso tratta come il codice del file. Le altre (il resto del testo, i metadati) sono evidenze piu' deboli.
func (e EvidenzaContenutoPDF) DalCartiglio() bool {
	return e.Fonte == FontePDFCartiglio || e.Fonte == FontePDFOCR
}

// EvidenzeContenutoPDF sono le evidenze dal contenuto di un PDF della RFQ, normalizzate con il Motore del
// cliente: la funzione che il flusso chiama per ogni PDF (LeggiStatoFlusso). Oggi e' vuota: la lettura
// strutturata dei PDF (F9) la riempie dopo l'unione, dai suoi fatti e con la sua normalizzazione. Non scrive
// niente.
func EvidenzeContenutoPDF(ctx context.Context, q *db.Queries, sha string, m *classificazione.Motore) ([]EvidenzaContenutoPDF, error) {
	return nil, nil
}
