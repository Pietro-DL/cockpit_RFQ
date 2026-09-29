package fascicolo

// La porta del testo dei PDF (Smistamento F9) verso il flusso ancorato al prodotto (F8): giro 4, fase 4.2.
//
// La separazione e' quella degli STEP (precisazione dell'utente del 27/09 sera): il worker estrae FATTI dal
// contenuto (analisi_fatti, `testo_pdf`), la classificazione li normalizza in LETTURE (classificazione.
// LettureDelPDF, per le regole del cliente della RFQ e per il nome di QUESTO allegato), e il flusso consuma
// solo le evidenze che ne vengono, da qui e da nessun'altra parte: non piu' anche dalla valutazione del file. Il
// flusso non legge mai il JSON del worker: ne' quello degli STEP (usa le righe di componente_proposta e
// relazione_proposta che ApplicaStruttura ne ha tratto) ne' quello dei PDF. Un codice letto nel contenuto di un
// PDF e' un'evidenza indipendente dal nome del file, come la radice di uno STEP (decisioni del 27/09 «ter»), con
// i limiti di ogni fonte (codiciDelFile):
//   - il cartiglio (testo nativo in basso a destra della pagina 1) con un codice di FAMIGLIA del cliente e'
//     contenuto; se ripete soltanto il nome del file dipende dal nome (Domanda 7 = B) e non e' una seconda
//     fonte; un codice generico li' e' solo una chiave di ricerca (P33);
//   - i metadati (titolo, soggetto) non sostengono mai una destinazione (A-P2, «solo_metadati»);
//   - il resto del testo e' una chiave di ricerca (P33);
//   - l'OCR e' un indizio: nessun candidato, solo la frase (27/09 «ter»: un'evidenza, non un automatismo);
//   - il «particolare simile» non e' un codice: e' la nota «simile a X» (risposta 4 del 29/09); lo stesso per
//     «SPECULARE DI X», la nota «speculare di X» (fase 4.6);
//   - nella zona del cartiglio conta solo il campo del codice: le righe dell'elenco particolari di un disegno
//     d'assieme non sono letture ne' chiavi (fase 4.6, classificazione.EvidenzeTestoPDF);
//   - il cartiglio di un testo letto con il worker di prima (una sottoversione del testo di prima di quella di
//     oggi) non e' contenuto: i suoi codici sono chiavi di ricerca, come il resto del testo, con la frase
//     «testo letto con il worker di prima: da rianalizzare», finche' «Rianalizza» non lo rilegge (fase 4.6r,
//     EvidenzaContenutoPDF.DaRileggere).
//
// Le stesse letture danno l'ancora «piatta» del prodotto (A-P1, A-P2, A-P3: smistamento_ancora.go), e lo
// stato del testo (letto, senza testo, non letto, illeggibile, da analizzare) con la sua frase: un PDF il cui
// testo non c'e' non ancora niente, e lo dice.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// Le fonti di un'evidenza dal contenuto di un PDF: dove, nel file, e' stato letto il codice. Sono le fonti
// delle letture della classificazione (una porta sola: nessun nome diverso per la stessa cosa).
const (
	FontePDFCartiglio = classificazione.FonteTestoCartiglio // testo nativo in basso a destra della pagina 1
	FontePDFTesto     = classificazione.FonteTestoPagina    // testo nativo altrove
	FontePDFMetadati  = classificazione.FonteMetadatiPDF    // titolo o soggetto del documento
	FontePDFOCR       = classificazione.FonteOCR            // letto con l'OCR: un indizio, mai «dal contenuto»
)

// L'origine di un codice letto: una famiglia del cliente, o l'estrattore generico (la forma di un codice).
const (
	OrigineFamiglia = "famiglia"
	OrigineGenerico = "generico"
)

// EvidenzaContenutoPDF e' un codice letto DENTRO un PDF, gia' normalizzato dal core (una
// classificazione.LetturaTesto nella forma che il flusso consuma). Il codice e' quello che il Motore delle
// regole del cliente della RFQ riconosce; la pagina e la posizione dicono dove, per mostrarlo.
type EvidenzaContenutoPDF struct {
	Fonte     string       `json:"fonte"` // FontePDF*
	Codice    string       `json:"codice"`
	Rev       string       `json:"rev,omitempty"`
	Pagina    int          `json:"pagina,omitempty"` // da 1; 0 = non si sa (i metadati)
	Posizione *RiquadroPDF `json:"posizione,omitempty"`
	Famiglia  string       `json:"famiglia,omitempty"` // la famiglia del cliente che ha riconosciuto il codice
	// Origine: OrigineFamiglia o OrigineGenerico. Nel cartiglio solo un codice di famiglia e' il codice del
	// file; un generico e' una chiave di ricerca (P33).
	Origine   string `json:"origine,omitempty"`
	Etichetta string `json:"etichetta,omitempty"` // il campo del cartiglio in cui sta, se sta in un campo
	// DipendeDaNome: la lettura ripete il nome del file (Domanda 7 = B): resta, ma non e' una seconda fonte.
	DipendeDaNome bool `json:"dipende_da_nome,omitempty"`
	// Indizio: letta con l'OCR. Non da' un candidato ne' un'ancora; si dice soltanto.
	Indizio bool `json:"indizio,omitempty"`
	// DaRileggere: letta nel cartiglio di un testo della sottoversione di prima (classificazione.LetturaTesto.
	// DaRileggere, fase 4.6r): una chiave di ricerca, mai il codice del file ne' un sostegno, finche' «Rianalizza»
	// non lo rilegge.
	DaRileggere bool `json:"da_rileggere,omitempty"`
}

// RiquadroPDF e' la posizione di un'evidenza nella pagina, in punti, con l'origine in alto a sinistra.
type RiquadroPDF struct {
	X0 float64 `json:"x0"`
	Y0 float64 `json:"y0"`
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
}

// DalCartiglio dice se l'evidenza e' del cartiglio letto nel testo nativo: la lettura che il flusso tratta
// come il codice del file. Le altre (il resto del testo, i metadati) sono evidenze piu' deboli; l'OCR, anche
// della zona del cartiglio, e' un indizio e non conta (decisioni del 27/09 «ter»: un'evidenza, non un
// automatismo; fino alla calibrazione S2 non ha uno score, e non sostiene ne' preseleziona niente). Nemmeno il
// cartiglio di un testo letto con il worker di prima (DaRileggere, fase 4.6r).
func (e EvidenzaContenutoPDF) DalCartiglio() bool {
	return e.Fonte == FontePDFCartiglio && !e.Indizio && !e.DaRileggere
}

// TestoDelPDF e' lo stato del testo di un PDF come lo vede il flusso: lo stato (classificazione.Testo…), la sua
// frase (classificazione.FraseTestoPDF, "" per un testo letto dal worker di oggi), l'esito dell'OCR, se il testo e'
// di una sottoversione di prima (DaRileggere, fase 4.6r: la frase e' allora classificazione.FraseTestoDiPrima) e i
// codici del particolare simile e dello speculare (le note «simile a X» e «speculare di X»). Vuoto per un file che
// non e' un PDF.
type TestoDelPDF struct {
	Stato       string   `json:"stato,omitempty"`
	Frase       string   `json:"frase,omitempty"`
	OCR         string   `json:"ocr,omitempty"`
	DaRileggere bool     `json:"da_rileggere,omitempty"`
	Simili      []string `json:"simili,omitempty"`
	Speculari   []string `json:"speculari,omitempty"`
}

// Letto dice se il testo del PDF c'e': i fatti correnti lo portano, con del testo nativo.
func (t TestoDelPDF) Letto() bool { return t.Stato == classificazione.TestoLetto }

// evidenzeDalleLetture e' la normalizzazione delle letture di un PDF nella forma del flusso: lo stato del testo
// e le evidenze, nell'ordine delle letture (cartiglio, metadati, resto del testo, OCR). Pura: e' la sola
// strada dalle letture al flusso.
func evidenzeDalleLetture(l classificazione.LettureTestoPDF) (TestoDelPDF, []EvidenzaContenutoPDF) {
	t := TestoDelPDF{Stato: l.Stato, Frase: l.Frase, OCR: l.OCR, DaRileggere: l.DaRileggere, Simili: append([]string(nil), l.Simili...),
		Speculari: append([]string(nil), l.Speculari...)}
	var out []EvidenzaContenutoPDF
	for _, x := range l.Letture {
		e := EvidenzaContenutoPDF{Fonte: x.Fonte, Codice: strings.ToUpper(strings.TrimSpace(x.Codice)), Rev: x.Rev, Pagina: x.Pagina,
			Famiglia: x.Famiglia, Origine: OrigineGenerico, Etichetta: x.Etichetta, DipendeDaNome: x.DipendeDaNome, Indizio: x.Indizio(),
			DaRileggere: x.DaRileggere}
		if x.Origine == OrigineFamiglia {
			e.Origine = OrigineFamiglia
		}
		if len(x.Riquadro) == 4 {
			e.Posizione = &RiquadroPDF{X0: x.Riquadro[0], Y0: x.Riquadro[1], X1: x.Riquadro[2], Y1: x.Riquadro[3]}
		}
		if e.Codice != "" {
			out = append(out, e)
		}
	}
	return t, out
}

// pdfAnalizzato dice se un allegato e' un PDF che un'analisi ha gia' letto (con un analizzatore qualunque):
// senza i fatti correnti, il suo testo e' «non letto, da rianalizzare», non «da analizzare».
func pdfAnalizzato(estensione, sha string, stato db.StatoAllegato) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(estensione), "."), "pdf") && sha != "" &&
		stato == db.StatoAllegatoAnalizzato
}

// lettureCorrenti sono le letture normalizzate del testo di un PDF dai fatti dell'analizzatore CORRENTE
// (classificazione.LettureDelPDF), per le regole del cliente della RFQ (m) e il nome di QUESTO allegato: la
// funzione comune di TestoCorrenteDelPDF e di EvidenzeContenutoPDF. Senza i fatti correnti non si indovina: un
// PDF gia' analizzato da un analizzatore precedente (giaAnalizzato) e' «testo non letto, da rianalizzare» (come
// uno con i fatti correnti ma senza il testo, che dice LettureDelPDF), uno non ancora analizzato e' «da
// analizzare». Legge soltanto.
func lettureCorrenti(ctx context.Context, q *db.Queries, sha, nome string, giaAnalizzato bool, an coda.Analizzatore,
	m *classificazione.Motore) (classificazione.LettureTestoPDF, error) {
	solo := func(stato string) classificazione.LettureTestoPDF {
		return classificazione.LettureTestoPDF{Stato: stato, Frase: classificazione.FraseTestoPDF(stato, "")}
	}
	if sha == "" {
		return solo(classificazione.TestoDaAnalizzare), nil
	}
	f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: sha, VersioneAnalizzatore: int16(an.Versione),
		HashConfigurazione: an.Hash()})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if giaAnalizzato {
			return solo(classificazione.TestoNonLetto), nil
		}
		return solo(classificazione.TestoDaAnalizzare), nil
	case err != nil:
		return classificazione.LettureTestoPDF{}, err
	}
	return classificazione.LettureDelPDF(m, f.Fatti, nome), nil
}

// EvidenzeContenutoPDF sono lo stato del testo e le evidenze dal contenuto di un PDF della RFQ, per il nome di
// QUESTO allegato (P15, Domanda 7 = B), normalizzate con il Motore del cliente dai fatti dell'analizzatore
// corrente: la porta da cui il testo dei PDF entra nel flusso (LeggiStatoFlusso). giaAnalizzato: l'allegato e'
// un PDF gia' letto da un'analisi qualunque (pdfAnalizzato). Non scrive niente.
func EvidenzeContenutoPDF(ctx context.Context, q *db.Queries, sha, nome string, giaAnalizzato bool, an coda.Analizzatore,
	m *classificazione.Motore) (TestoDelPDF, []EvidenzaContenutoPDF, error) {
	l, err := lettureCorrenti(ctx, q, sha, nome, giaAnalizzato, an, m)
	if err != nil {
		return TestoDelPDF{}, nil, err
	}
	t, ev := evidenzeDalleLetture(l)
	return t, ev, nil
}
