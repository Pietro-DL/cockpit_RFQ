package classificazione

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/contratti/worker"
)

// Le prove del testo dei PDF sul server (Smistamento F9, A5.13.8; righe pdf_* della tabella S1, A5.14.3; P33;
// Domanda 7 = B; decisioni del 27/09 «ter»). Il worker riporta i fatti del testo senza codici (frammenti con
// pagina e riquadro, campi del cartiglio, metadati, OCR); i codici li cerca il Motore del cliente; le letture
// normalizzate sono quello che esce verso il flusso (F8); nella valutazione entrano solo il codice di famiglia
// del testo nativo in basso a destra della pagina 1 e il titolo o il soggetto dei metadati, e l'OCR come
// indizio senza voto.

// riquadroBD e riquadroAlto sono due posti finti sulla pagina 1 (A4 orizzontale): in basso a destra e in alto a
// sinistra.
var (
	riquadroBD   = []float64{560, 530, 655, 543}
	riquadroAlto = []float64{40, 50, 200, 63}
)

// testoPDF e' il testo di un disegno finto: un frammento nativo in basso a destra della pagina 1 e uno nel resto
// della pagina.
func testoPDF(bassoDestra, pagina string, meta worker.MetadatiPDF) worker.TestoPDF {
	t := worker.TestoPDF{Versione: 1, Pagine: 1, PagineLette: 1, Metadati: meta, FormatoPagina1: []float64{842, 595},
		OCR:    worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Limiti: worker.LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384}}
	if bassoDestra != "" {
		t.Frammenti = append(t.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: worker.ZonaBassoDestra,
			Fonte: worker.FonteTestoNativo, Testo: bassoDestra, Riquadro: riquadroBD})
		t.Caratteri += len(bassoDestra)
	}
	if pagina != "" {
		t.Frammenti = append(t.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: worker.ZonaPagina,
			Fonte: worker.FonteTestoNativo, Testo: pagina, Riquadro: riquadroAlto})
		t.Caratteri += len(pagina)
	}
	t.Estraibile = t.Caratteri > 0
	return t
}

// conOCR aggiunge al testo un frammento letto dall'OCR (con la sua confidenza) e l'esito dell'OCR.
func conOCR(t worker.TestoPDF, pagina int, zona, testo string, confidenza float64) worker.TestoPDF {
	c := confidenza
	t.Frammenti = append(t.Frammenti, worker.FrammentoPDF{Pagina: pagina, Zona: zona, Fonte: worker.FonteTestoOCR,
		Testo: testo, Riquadro: []float64{600, 500, 700, 512}, Confidenza: &c})
	t.OCR = worker.OCRPDF{Stato: worker.OCREseguito, Motore: "finto",
		Tentativi: []worker.TentativoOCR{{Pagina: pagina, Zona: zona, Esito: "letto", Caratteri: len(testo)}}}
	return t
}

// fattiDisegno sono i fatti dell'analizzatore 4 per un PDF con i termini del cartiglio e il suo testo.
func fattiDisegno(t *testing.T, tp worker.TestoPDF) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"},
		"fonti": map[string]string{"tipo": "termini_pdf"}, "testo_pdf": tp})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// valutaDisegno valuta un PDF di disegno dell'analisi, con i fatti dati, per il cliente ACME.
func valutaDisegno(t *testing.T, nome string, fatti json.RawMessage) Valutazione {
	t.Helper()
	return Valuta(IngressoFile{Da: DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: motoreACME(t),
		Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: fatti})
}

func evidenzaDi(d Dimensione, regola string) (Evidenza, bool) {
	for _, e := range d.Evidenze {
		if e.Regola == regola {
			return e, true
		}
	}
	return Evidenza{}, false
}

// TestEvidenzeTestoPDF (F9, A5.13.8, P33): i codici del testo, fonte per fonte, con le regole del cliente. Nel
// testo nativo in basso a destra quelli di famiglia e i generici (le ancore piatte A-P1 li vogliono tutti e
// due), con il riquadro e, quando stanno in un campo del cartiglio, l'etichetta e la rev del cartiglio; nei
// metadati il titolo che E' un codice e i codici di famiglia citati; nel resto delle pagine gli altri, senza
// ripetere quelli gia' visti; l'OCR in una lista sua. Il riferimento della richiesta non e' un codice; senza
// famiglie resta l'estrattore generico; Chiavi li mette in fila per l'indice, senza l'OCR.
func TestEvidenzeTestoPDF(t *testing.T) {
	tp := testoPDF("DISEGNO N. 7120010\nSCALA 1:2 REV. B\nORDINE 4500012345",
		"NOTA 7120011 VEDERE CAPITOLATO\nMATERIALE S235JR 7120012",
		worker.MetadatiPDF{Titolo: "7120010_1.pdf", Soggetto: "Staffa 7120012 per ACME", Creatore: "CAD"})
	campoDis := []float64{620, 530, 655, 543}
	tp.Cartiglio = []worker.CampoCartiglio{
		{Etichetta: worker.CampoNumeroDisegno, Letta: "DISEGNO N.", Valore: "7120010", Pagina: 1, Zona: worker.ZonaBassoDestra,
			Fonte: worker.FonteTestoNativo, Riquadro: campoDis},
		{Etichetta: worker.CampoRevisione, Letta: "REV.", Valore: "B", Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo},
		{Etichetta: worker.CampoScala, Letta: "SCALA", Valore: "1:2", Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo},
	}
	tp = conOCR(tp, 2, worker.ZonaPagina, "PARTICOLARE 7120099 e 7120011", 63)
	e := EvidenzeTestoPDF(motoreACME(t), tp)
	if !e.Estraibile {
		t.Error("il testo c'e'")
	}
	type atteso struct{ codice, origine, zona, fonte string }
	vedi := func(l []LetturaTesto) []atteso {
		var out []atteso
		for _, c := range l {
			out = append(out, atteso{c.Codice, c.Origine, c.Zona, c.Fonte})
		}
		return out
	}
	if got, want := vedi(e.Cartiglio), []atteso{{"7120010", "famiglia", ZonaBassoDestra, FonteTestoCartiglio},
		{"4500012345", "generico", ZonaBassoDestra, FonteTestoCartiglio}}; !reflect.DeepEqual(got, want) {
		t.Errorf("in basso a destra: %+v, attesi %+v", got, want)
	}
	if c := e.Cartiglio[0]; c.Pagina != 1 || c.Dove != DoveBassoDestra || c.Estratto != "DISEGNO N. 7120010" || c.Famiglia != "ACME 712" ||
		c.Etichetta != worker.CampoNumeroDisegno || !reflect.DeepEqual(c.Riquadro, campoDis) || c.RevCartiglio != "B" || c.Indizio() {
		t.Errorf("il codice del cartiglio dice dove sta, in che campo, con che rev: %+v", c)
	}
	if c := e.Cartiglio[1]; c.Etichetta != "" || !reflect.DeepEqual(c.Riquadro, riquadroBD) {
		t.Errorf("un codice fuori dai campi ha il riquadro del suo frammento: %+v", c)
	}
	if got, want := vedi(e.Metadati), []atteso{{"7120010", "famiglia", ZonaTitolo, FonteMetadatiPDF},
		{"7120012", "famiglia", ZonaSoggetto, FonteMetadatiPDF}}; !reflect.DeepEqual(got, want) {
		t.Errorf("metadati: %+v, attesi %+v", got, want)
	}
	if !e.Metadati[0].Intero || e.Metadati[1].Intero || e.Metadati[0].Riquadro != nil || e.Metadati[0].Pagina != 0 {
		t.Errorf("il titolo E' il codice, il soggetto lo cita; i metadati non hanno un posto sulla pagina: %+v", e.Metadati)
	}
	// nel resto delle pagine: 7120011 (famiglia) e i generici non ancora visti (S235JR ha la forma di un codice);
	// 7120010, 7120012 e 4500012345 no
	if got, want := vedi(e.Altrove), []atteso{{"7120011", "famiglia", ZonaPagina, FonteTestoPagina},
		{"S235JR", "generico", ZonaPagina, FonteTestoPagina}}; !reflect.DeepEqual(got, want) {
		t.Errorf("il resto del testo: %+v, attesi %+v", got, want)
	}
	if e.Altrove[0].RevCartiglio != "" || !reflect.DeepEqual(e.Altrove[0].Riquadro, riquadroAlto) {
		t.Errorf("la rev del cartiglio non passa al resto del testo: %+v", e.Altrove[0])
	}
	// l'OCR: una lista sua, indizi con la confidenza del motore, anche per un codice gia' visto nel testo nativo
	if got, want := vedi(e.OCR), []atteso{{"7120099", "famiglia", ZonaPagina, FonteOCR}, {"7120011", "famiglia", ZonaPagina, FonteOCR}}; !reflect.DeepEqual(got, want) {
		t.Errorf("OCR: %+v, attesi %+v", got, want)
	}
	if c := e.OCR[0]; !c.Indizio() || c.Pagina != 2 || c.Confidenza == nil || *c.Confidenza != 63 || !strings.Contains(c.Dove, "OCR") {
		t.Errorf("una lettura dell'OCR e' un indizio, con la confidenza e la fonte detta: %+v", c)
	}
	if k := e.Chiavi(); !reflect.DeepEqual(k, []string{"7120010", "4500012345", "7120012", "7120011", "S235JR"}) {
		t.Errorf("chiavi di ricerca, nell'ordine delle fonti, senza ripetizioni e senza l'OCR: %v", k)
	}

	// senza le regole del cliente resta l'estrattore generico: niente famiglia, e i codici ci sono lo stesso
	g := EvidenzeTestoPDF(nil, tp)
	if len(g.Cartiglio) == 0 || g.Cartiglio[0].Codice != "7120010" || g.Cartiglio[0].Origine != "generico" {
		t.Errorf("senza famiglie: %+v", g.Cartiglio)
	}

	// un campo del cartiglio il cui frammento e' rimasto fuori dai limiti porta lo stesso il suo codice
	solo := testoPDF("", "", worker.MetadatiPDF{})
	solo.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "CODICE", Valore: "7120012", Pagina: 1,
		Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: campoDis}}
	if c := EvidenzeTestoPDF(motoreACME(t), solo).Cartiglio; len(c) != 1 || c[0].Codice != "7120012" || c[0].Etichetta != worker.CampoCodice {
		t.Errorf("il codice di un campo senza frammento: %+v", c)
	}

	// due rev diverse nel cartiglio: nessuna si sceglie
	due := tp
	due.Cartiglio = append(append([]worker.CampoCartiglio(nil), tp.Cartiglio...), worker.CampoCartiglio{Etichetta: worker.CampoRevisione,
		Valore: "C", Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo})
	if c := EvidenzeTestoPDF(motoreACME(t), due).Cartiglio[0]; c.RevCartiglio != "" {
		t.Errorf("con due rev diverse nel cartiglio la rev non si sceglie: %+v", c)
	}

	// un PDF senza testo: nessun codice dal testo, ma i metadati parlano ancora (A-P2)
	muto := testoPDF("", "", worker.MetadatiPDF{Titolo: "7120010"})
	m := EvidenzeTestoPDF(motoreACME(t), muto)
	if m.Estraibile || len(m.Cartiglio) != 0 || len(m.Altrove) != 0 || len(m.Metadati) != 1 || !m.Metadati[0].Intero {
		t.Errorf("PDF senza testo: %+v", m)
	}
}

// TestLettureDelPDF (F9 → F8; decisioni del 27/09, precisazioni): il flusso non legge mai il JSON del worker.
// LettureDelPDF normalizza i fatti in letture con la fonte (testo nativo del cartiglio, testo altrove, metadati,
// OCR), il codice, la rev del cartiglio, la pagina, il riquadro e la dipendenza dal nome del file (Domanda 7 =
// B), e dice lo stato del testo con la sua frase e l'esito dell'OCR.
func TestLettureDelPDF(t *testing.T) {
	acme := motoreACME(t)
	tp := testoPDF("DISEGNO N. 7120010\nC:\\DISEGNI\\7120011.pdf", "NOTA 7120012", worker.MetadatiPDF{Titolo: "7120011"})
	tp.Troncato = true
	tp = conOCR(tp, 1, worker.ZonaBassoDestra, "DISEGNO N. 7120011", 58)
	l := LettureDelPDF(acme, fattiDisegno(t, tp), "7120011.pdf")
	if l.Stato != TestoLetto || l.Frase != "" || l.OCR != worker.OCREseguito || !l.Troncato {
		t.Errorf("stato del testo letto: %+v", l)
	}
	type atteso struct {
		fonte, codice string
		dipende       bool
	}
	var got []atteso
	for _, x := range l.Letture {
		got = append(got, atteso{x.Fonte, x.Codice, x.DipendeDaNome})
	}
	want := []atteso{
		{FonteTestoCartiglio, "7120010", false}, // diverso dal nome: non dipende, e' la discordanza
		{FonteTestoCartiglio, "7120011", true},  // in basso a destra solo dentro il nome del file stampato
		{FonteMetadatiPDF, "7120011", true},     // il titolo uguale al nome dipende dal nome
		{FonteTestoPagina, "7120012", false},
		{FonteOCR, "7120011", false}, // l'OCR l'ha letto scritto per conto suo: un indizio indipendente
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("letture:\n %+v\natteso\n %+v", got, want)
	}
	if k := l.Chiavi(); !reflect.DeepEqual(k, []string{"7120010", "7120011", "7120012"}) {
		t.Errorf("chiavi: %v", k)
	}
	// un codice letto SOLO dall'OCR e' una lettura (un indizio, con la sua fonte) ma non una chiave di ricerca:
	// sopra il codice dell'OCR c'e' anche fra i nativi, e la deduplicazione nasconderebbe un'OCR che entra
	solo := LettureDelPDF(acme, fattiDisegno(t, conOCR(tp, 2, worker.ZonaPagina, "PARTICOLARE 7120099", 40)), "7120011.pdf")
	ocrSolo := false
	for _, x := range solo.Letture {
		if x.Codice == "7120099" {
			ocrSolo = x.Fonte == FonteOCR && x.Indizio()
		}
	}
	if !ocrSolo {
		t.Errorf("il codice letto solo dall'OCR e' una lettura OCR, un indizio: %+v", solo.Letture)
	}
	if k := solo.Chiavi(); slices.Contains(k, "7120099") {
		t.Errorf("il codice letto solo dall'OCR non e' una chiave di ricerca: %v", k)
	}
	// la stessa lettura per un altro nome (P15): la dipendenza e' del nome di QUESTO allegato
	for _, x := range LettureDelPDF(acme, fattiDisegno(t, tp), "tavola.pdf").Letture {
		if x.DipendeDaNome {
			t.Errorf("un file che nel nome non ha un codice non ha letture che ne dipendono: %+v", x)
		}
	}

	casi := []struct {
		nome, fatti, stato, frase, ocr string
	}{
		{"senza testo, OCR assente", `{"testo_pdf": {"versione": 1, "estraibile": false, "ocr": {"stato": "non_disponibile", "motivo": "Tesseract non disponibile"}}}`,
			TestoAssente, "serve l'OCR, che sul worker non c'e'", worker.OCRNonDisponibile},
		{"senza testo, OCR eseguito", `{"testo_pdf": {"versione": 1, "estraibile": false, "ocr": {"stato": "eseguito"}}}`,
			TestoAssente, "letto con l'OCR, solo come indizio", worker.OCREseguito},
		// giro di correzione 2: la frase non dice piu' soltanto «di prima dell'analizzatore 4», falso per un fatto v4
		// senza testo (worker non aggiornato); «da rianalizzare» e' vero per tutti e due, perche' «Rianalizza» li
		// riaccoda tutti e due (fascicolo.AccodaPdfDaRileggere)
		{"fatti v3 o di un worker vecchio", `{"codice_riconosciuto": "7120010", "testo_letto": 12}`, TestoNonLetto, "o di un worker non aggiornato): da rianalizzare", ""},
		{"illeggibile", `{"errore_pdf": "cannot open broken document"}`, TestoIlleggibile, "non si apre", ""},
	}
	for _, c := range casi {
		l := LettureDelPDF(acme, json.RawMessage(c.fatti), "7120010.pdf")
		if l.Stato != c.stato || !strings.Contains(l.Frase, c.frase) || l.OCR != c.ocr || len(l.Letture) != 0 {
			t.Errorf("%s: %+v", c.nome, l)
		}
	}
	if l := LettureDelPDF(acme, json.RawMessage(`{"testo_pdf": {"versione": 1, "estraibile": false, "ocr": {"stato": "non_disponibile", "motivo": "x"}}}`), "a.pdf"); l.MotivoOCR != "x" {
		t.Errorf("il motivo dell'OCR passa: %+v", l)
	}
}

// TestIlCartiglioConfermaIlNome (F9, A5.14.3; Domanda 7 = B): il codice di famiglia in basso a destra della
// pagina 1 uguale al codice del nome e' una conferma INDIPENDENTE: due fonti concordi, e vince la lettura del
// contenuto (pdf_testo_famiglia 85, fonte cartiglio). La rev resta quella del nome (la rev del cartiglio non ha
// una riga in S1: Domanda 3 = A). La valutazione resta piccola.
func TestIlCartiglioConfermaIlNome(t *testing.T) {
	tp := testoPDF("TOLLERANZE GENERALI\nDISEGNO N. 7120010\nSCALA 1:2 REV. B", "NOTA 12345", worker.MetadatiPDF{})
	tp.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoRevisione, Letta: "REV.", Valore: "B", Pagina: 1,
		Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo}}
	v := valutaDisegno(t, "7120010_1.pdf", fattiDisegno(t, tp))
	controllaDim(t, "cartiglio concorde", "codice", v.Codice, dimAttesa{"7120010", 85, "pdf_testo_famiglia", StatoConcorde})
	controllaDim(t, "cartiglio concorde", "rev", v.Rev, dimAttesa{"1", 40, "rev_suffisso_nome", StatoUnica})
	controllaDim(t, "cartiglio concorde", "tipo", v.Tipo, dimAttesa{"disegno_2d", 75, "pdf_termini_cartiglio", StatoUnica})
	e, ok := evidenzaDi(v.Codice, "pdf_testo_famiglia")
	if !ok || e.DipendeDa != "" || e.Fonte != "cartiglio" || e.Dove != DoveBassoDestra || e.Famiglia != "ACME 712" || e.Testo != "DISEGNO N. 7120010" {
		t.Errorf("la lettura del cartiglio: %+v", e)
	}
	if r := v.Riepilogo(); r != (Riepilogo{"disegno_2d", "7120010", "1", 85, "cartiglio"}) {
		t.Errorf("colonne: %+v", r)
	}
	if b, _ := json.Marshal(v); len(b) > 900 {
		t.Errorf("la valutazione deve stare sotto i 900 byte (A5.14.2): %d", len(b))
	}
}

// TestUnaLetturaDelTestoCheRipeteIlNomeNonAlzaLoScore (F9; Domanda 7 = B): il titolo dei metadati uguale al
// nome e un codice che in basso a destra sta solo dentro il nome del file ripetuto («7120010_1.pdf» stampato
// sotto il disegno) DIPENDONO dal nome: restano fra le evidenze, non fanno una seconda fonte e non valgono piu'
// del nome. Il codice resta quello del nome, con la sua regola e il suo score.
func TestUnaLetturaDelTestoCheRipeteIlNomeNonAlzaLoScore(t *testing.T) {
	casi := []struct {
		nome, file  string
		tp          worker.TestoPDF
		codice      dimAttesa
		regola      string
		score, orig int
	}{
		{"titolo uguale al nome generico", "7120010A_1.pdf",
			testoPDF("", "TOLLERANZE GENERALI", worker.MetadatiPDF{Titolo: "7120010A_1.pdf"}),
			dimAttesa{"7120010A", 45, "nome_codice_generico", StatoUnica}, "pdf_metadati", 40, 0},
		{"titolo uguale al nome di famiglia", "7120010.pdf",
			testoPDF("", "TOLLERANZE GENERALI", worker.MetadatiPDF{Titolo: "7120010", Soggetto: "7120010"}),
			dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, "pdf_metadati", 40, 0},
		{"il nome del file stampato in basso a destra", "7120010_1.pdf",
			testoPDF("SCALA 1:2\nC:\\DISEGNI\\7120010_1.pdf", "", worker.MetadatiPDF{}),
			dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, "pdf_testo_famiglia", 70, 85},
		{"il nome senza estensione stampato in basso a destra", "7120010_1.pdf",
			testoPDF("SCALA 1:2\n7120010_1", "", worker.MetadatiPDF{}),
			dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, "pdf_testo_famiglia", 70, 85},
	}
	for _, c := range casi {
		v := valutaDisegno(t, c.file, fattiDisegno(t, c.tp))
		controllaDim(t, c.nome, "codice", v.Codice, c.codice)
		e, ok := evidenzaDi(v.Codice, c.regola)
		if !ok {
			t.Errorf("%s: la lettura del testo resta registrata: %+v", c.nome, v.Codice.Evidenze)
			continue
		}
		if e.DipendeDa != "nome_file" || e.Score != c.score || e.ScoreRegola != c.orig {
			t.Errorf("%s: la lettura del testo dipende dal nome: %+v", c.nome, e)
		}
		if n := strings.Count(mustJSON(t, v.Codice.Evidenze), `"regola":"pdf_metadati"`); n > 1 {
			t.Errorf("%s: titolo e soggetto con lo stesso codice sono una lettura sola: %d", c.nome, n)
		}
		if r := v.Riepilogo(); r.Codice != c.codice.valore || r.Confidenza != c.codice.score || r.Fonte != "nome_file" {
			t.Errorf("%s: colonne %+v", c.nome, r)
		}
	}

	// il rovescio: lo stesso nome stampato E il codice scritto nel cartiglio. Il cartiglio conferma: due fonti
	v := valutaDisegno(t, "7120010_1.pdf", fattiDisegno(t, testoPDF("DISEGNO N. 7120010\nC:\\DISEGNI\\7120010_1.pdf", "",
		worker.MetadatiPDF{})))
	controllaDim(t, "nome stampato e cartiglio", "codice", v.Codice, dimAttesa{"7120010", 85, "pdf_testo_famiglia", StatoConcorde})

	// la stessa regola per chi usa il cartiglio fuori dalla valutazione (il flusso ancorato, A-P1, P32), dalle
	// letture normalizzate. Riscritta nel giro di correzione 2 di F9: prima fissava NelCartiglioSoloIlNome, che
	// prendeva il fatto del worker (worker.TestoPDF) e non c'e' piu': F8 non legge il fatto, legge LettureDelPDF.
	acme := motoreACME(t)
	stampato := fattiDisegno(t, testoPDF("SCALA 1:2\nC:\\DISEGNI\\7120010_1.pdf", "", worker.MetadatiPDF{}))
	scritto := fattiDisegno(t, testoPDF("DISEGNO N. 7120010\nC:\\DISEGNI\\7120010_1.pdf", "", worker.MetadatiPDF{}))
	nelCartiglio := func(fatti json.RawMessage, nomeFile string) (letto, dipende bool) {
		t.Helper()
		for _, x := range LettureDelPDF(acme, fatti, nomeFile).Letture {
			if x.Fonte == FonteTestoCartiglio && x.Codice == "7120010" {
				return true, x.DipendeDaNome
			}
		}
		return false, false
	}
	if l, d := nelCartiglio(stampato, "7120010_1.pdf"); !l || !d {
		t.Errorf("il codice che sta solo nel nome stampato e' il nome del file: letto %v, dipende %v", l, d)
	}
	if l, d := nelCartiglio(scritto, "7120010_1.pdf"); !l || d {
		t.Errorf("il codice scritto anche nel cartiglio non e' solo il nome: letto %v, dipende %v", l, d)
	}
	for _, altro := range []string{"tavola.pdf", "7120011.pdf"} {
		if l, d := nelCartiglio(stampato, altro); !l || d {
			t.Errorf("%s: il nome stampato di un ALTRO file e' contenuto, non il nome di questo (P15): letto %v, dipende %v", altro, l, d)
		}
	}
}

// TestIlCartiglioDiversoDalNomeEUnaDiscordanza (F9; A5.14.3, U7): il codice di famiglia in basso a destra
// diverso da quello del nome non dipende da niente: la dimensione e' discorde, nessuno la precompila, e la
// colonna resta la lettura del nome (niente E04 sotto un altro nome, D49): il nome non si corregge. Lo stesso
// per un titolo diverso.
func TestIlCartiglioDiversoDalNomeEUnaDiscordanza(t *testing.T) {
	v := valutaDisegno(t, "7120010.pdf", fattiDisegno(t, testoPDF("DISEGNO N. 7120011\nSCALA 1:1", "", worker.MetadatiPDF{})))
	controllaDim(t, "cartiglio diverso", "codice", v.Codice, dimAttesa{"7120011", 85, "pdf_testo_famiglia", StatoDiscorde})
	if e, _ := evidenzaDi(v.Codice, "pdf_testo_famiglia"); e.DipendeDa != "" {
		t.Errorf("un codice diverso dal nome non dipende dal nome: %+v", e)
	}
	if r := v.Riepilogo(); r != (Riepilogo{"disegno_2d", "7120010", "", 70, "nome_file"}) {
		t.Errorf("con il codice discorde la colonna resta il nome: %+v", r)
	}

	v = valutaDisegno(t, "7120010.pdf", fattiDisegno(t, testoPDF("", "SCALA 1:1", worker.MetadatiPDF{Titolo: "7120012"})))
	controllaDim(t, "titolo diverso", "codice", v.Codice, dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoDiscorde})
}

// TestLOcrEUnIndizio (F9; decisioni del 27/09 «ter»: l'OCR e' un'evidenza, non un automatismo): un codice di
// famiglia letto dall'OCR in basso a destra della pagina 1 si registra fra le evidenze del codice, con la sua
// fonte e senza score, ma non vota. Uguale al nome non fa `concorde`; diverso dal nome non fa `discorde`; su un
// file senza codice nel nome non da' un codice al file. Lo score e la colonna restano quelli senza OCR, e la
// tabella S1 non ha una riga per lui (Domanda 3 = A).
func TestLOcrEUnIndizio(t *testing.T) {
	scansione := func(testo string) worker.TestoPDF {
		return conOCR(testoPDF("", "", worker.MetadatiPDF{}), 1, worker.ZonaBassoDestra, testo, 66)
	}
	casi := []struct {
		nome, file, ocr string
		codice          dimAttesa
		dipende         string
	}{
		{"uguale al nome", "7120010.pdf", "DISEGNO N. 7120010", dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, ""},
		{"diverso dal nome", "7120010.pdf", "DISEGNO N. 7120011", dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, ""},
		{"il nome stampato", "7120010_1.pdf", "7120010_1.pdf", dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica}, "nome_file"},
		// senza valore la dimensione dice «nessuna evidenza», con la regola dell'indizio fra parentesi
		{"nome senza codice", "scansione.pdf", "DISEGNO N. 7120011", dimAttesa{"", 0, RegolaOCRIndizio, StatoNessuna}, ""},
	}
	for _, c := range casi {
		v := valutaDisegno(t, c.file, fattiDisegno(t, scansione(c.ocr)))
		controllaDim(t, c.nome, "codice", v.Codice, c.codice)
		e, ok := evidenzaDi(v.Codice, RegolaOCRIndizio)
		if !ok {
			t.Errorf("%s: l'indizio dell'OCR si registra: %+v", c.nome, v.Codice.Evidenze)
			continue
		}
		if e.Valore != "" || e.Score != 0 || e.Indizio == "" || !strings.Contains(e.Dove, "OCR") || e.DipendeDa != c.dipende || e.Famiglia != "ACME 712" {
			t.Errorf("%s: un indizio porta il codice fuori dal valore, senza score e con la fonte: %+v", c.nome, e)
		}
		if _, s1 := Punteggi[RegolaOCRIndizio]; s1 {
			t.Errorf("l'indizio dell'OCR non e' una riga della tabella S1")
		}
	}
	// un codice generico o fuori dalla zona del cartiglio, letto dall'OCR, non e' nemmeno un indizio del codice
	altrove := conOCR(testoPDF("", "", worker.MetadatiPDF{}), 2, worker.ZonaPagina, "DISEGNO N. 7120011", 66)
	if v := valutaDisegno(t, "7120010.pdf", fattiDisegno(t, altrove)); v.HaEvidenza(RegolaOCRIndizio) {
		t.Errorf("l'OCR di un'altra pagina resta una chiave per il flusso: %+v", v.Codice.Evidenze)
	}
	if Indizi[RegolaOCRIndizio].Parole == "" {
		t.Error("l'indizio si dice a parole nella schermata")
	}
	if ImprontaPunteggi != improntaTabella() {
		t.Error("la tabella S1 non cambia per l'OCR")
	}
}

// TestUnTokenDelTestoNonDiventaIlCodiceDelFile (F9, P33, A5.14.3): i codici generici in basso a destra, i
// codici del resto delle pagine e un numero citato nel titolo fra altre parole non votano. Servono solo a
// cercare nell'indice (EvidenzeTesto.Chiavi). Un file con un nome che non e' un codice resta senza codice.
func TestUnTokenDelTestoNonDiventaIlCodiceDelFile(t *testing.T) {
	tp := testoPDF("ORDINE 4500012345\nSCALA 1:2", "NOTA 7120011 VEDERE\nISO 2768-MK\nORDINE 4500012345",
		worker.MetadatiPDF{Titolo: "Offerta 4500012399 staffe"})
	v := valutaDisegno(t, "disegno staffa.pdf", fattiDisegno(t, tp))
	controllaDim(t, "solo chiavi", "codice", v.Codice, dimAttesa{"", 0, "", StatoNessuna})
	if len(v.Codice.Evidenze) != 0 {
		t.Errorf("nessuna lettura del codice dal corpo del testo: %+v", v.Codice.Evidenze)
	}
	if k := EvidenzeTestoPDF(motoreACME(t), tp).Chiavi(); !contieneTutte(k, "4500012345", "7120011", "4500012399") {
		t.Errorf("restano chiavi di ricerca: %v", k)
	}
	// senza le famiglie del cliente anche il codice in basso a destra e' generico: chiave, non codice del file
	v = Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "disegno staffa.pdf", Direzione: "entrata",
		Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: fattiDisegno(t, testoPDF("DISEGNO N. 7120010", "", worker.MetadatiPDF{}))})
	controllaDim(t, "senza famiglie", "codice", v.Codice, dimAttesa{"", 0, "", StatoNessuna})

	// un PDF senza testo dice il suo codice solo dai metadati, se il titolo e' un codice: pdf_metadati 40
	muto := json.RawMessage(`{"codice_riconosciuto": "", "testo_letto": 1,
		"testo_pdf": {"versione": 1, "estraibile": false, "frammenti": [], "metadati": {"titolo": "7120010"}}}`)
	v = Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "scansione.pdf", Direzione: "entrata", Motore: motoreACME(t),
		Esito: &Esito{"da_determinare", "estensione"}, Fatti: muto})
	controllaDim(t, "solo metadati", "codice", v.Codice, dimAttesa{"7120010", 40, "pdf_metadati", StatoUnica})
}

// TestUnFattoSenzaTestoPdfNonEUnPdfSenzaTesto (F9, prova 164 per la parte del worker; A5.13.3 passo 2b.4): un
// PDF letto senza testo e' TestoAssente, e la valutazione lo dice («serve l'OCR», con quello che l'OCR ha
// fatto); i fatti senza `testo_pdf` (analizzatore prima del 4, o un worker vecchio che ha risposto a un job v4)
// sono TestoNonLetto, e la valutazione dice «da rianalizzare» e non inventa niente; un PDF che non si apre e'
// TestoIlleggibile.
func TestUnFattoSenzaTestoPdfNonEUnPdfSenzaTesto(t *testing.T) {
	casi := []struct {
		nome, fatti string
		stato       string
		conTesto    bool
		frase       string
	}{
		{"letto", `{"testo_pdf": {"versione": 1, "estraibile": true, "frammenti": [{"pagina": 1, "zona": "pagina", "fonte": "nativo", "testo": "SCALA"}]}}`, TestoLetto, true, ""},
		{"senza testo", `{"testo_letto": 1, "testo_pdf": {"versione": 1, "estraibile": false, "frammenti": [], "ocr": {"stato": "non_disponibile"}}}`,
			TestoAssente, true, "serve l'OCR, che sul worker non c'e'"},
		{"senza testo, OCR spento", `{"testo_letto": 1, "testo_pdf": {"versione": 1, "estraibile": false, "ocr": {"stato": "spento"}}}`,
			TestoAssente, true, "serve l'OCR, spento sul worker"},
		{"fatto v4 di un worker vecchio", `{"codice_riconosciuto": "7120010", "testo_letto": 1}`, TestoNonLetto, false, "da rianalizzare"},
		{"versione sconosciuta", `{"testo_letto": 1, "testo_pdf": {"versione": 0, "estraibile": false}}`, TestoNonLetto, false, "da rianalizzare"},
		{"illeggibile", `{"errore_pdf": "cannot open broken document"}`, TestoIlleggibile, false, ""},
		{"niente", ``, TestoNonLetto, false, ""},
	}
	for _, c := range casi {
		tp, stato := testoDelPDF(json.RawMessage(c.fatti))
		if stato != c.stato || (tp != nil) != c.conTesto {
			t.Errorf("%s: stato %q, testo %v; atteso %q, %v", c.nome, stato, tp != nil, c.stato, c.conTesto)
		}
		if s := StatoDelTestoPDF(json.RawMessage(c.fatti)); s != c.stato {
			t.Errorf("%s: StatoDelTestoPDF %q, atteso %q", c.nome, s, c.stato)
		}
		if c.fatti == "" || c.stato == TestoLetto || c.stato == TestoIlleggibile {
			continue
		}
		v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "7120010.pdf", Direzione: "entrata", Motore: motoreACME(t),
			Esito: &Esito{"da_determinare", "nome_file"}, Fatti: json.RawMessage(c.fatti)})
		e, ok := evidenzaDi(v.Tipo, "pdf_nessun_termine")
		if !ok {
			t.Errorf("%s: il PDF letto senza termini: %+v", c.nome, v.Tipo)
			continue
		}
		if !strings.Contains(e.Testo, c.frase) || strings.Contains(e.Testo, "OCR") != (c.stato == TestoAssente) {
			t.Errorf("%s: «serve l'OCR» si dice solo di un PDF che il testo non ce l'ha, «da rianalizzare» di uno non letto: %q", c.nome, e.Testo)
		}
		controllaDim(t, c.nome, "codice", v.Codice, dimAttesa{"7120010", 70, "nome_codice_famiglia", StatoUnica})
	}
}

func mustJSON(t *testing.T, x any) string {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func contieneTutte(l []string, voci ...string) bool {
	for _, v := range voci {
		trovata := false
		for _, x := range l {
			if strings.EqualFold(x, v) {
				trovata = true
			}
		}
		if !trovata {
			return false
		}
	}
	return true
}
