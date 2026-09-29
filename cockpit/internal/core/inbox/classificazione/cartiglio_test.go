package classificazione

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/contratti/worker"
)

// Le prove del cartiglio letto meglio (giro 4, fase 4.6; piano del giro 4, la sezione «4.6» e quella dello scenario
// del 28/09 e dei bug della Distinta; risposta 4 del 29/09; domanda 3 del giro 4, A finche' l'utente non risponde): nella
// zona del cartiglio vota soltanto il campo del codice; il particolare simile riportato come campo e lo speculare
// sono note; la lettura del nome non esce dalle evidenze per il taglio; la sottoversione del testo dice che cosa va
// riletto. Tutti i dati sono inventati (ACME, 7120001…).

// testoAssieme e' il testo di un disegno d'assieme ACME inventato con la forma dei disegni del cliente del 28/09,
// come lo riporta il worker della sottoversione 2: nella zona in basso a destra della pagina 1, nell'ordine del file,
// l'ELENCO PARTICOLARI («Pos. Part Number Descrizione Q.ty» e le righe dei figli), «SPECULARE DI <speculare>», la
// tabella delle revisioni e il cartiglio con le etichette su una riga e i valori sotto; i campi sono quello del
// codice («Part Nr:», il codice dell'assieme) e il titolo. Nessun campo dall'intestazione dell'elenco (fase 4.6).
func testoAssieme(codice, speculare string, figli []string) worker.TestoPDF {
	var b strings.Builder
	b.WriteString("Pos. Part Number Descrizione Q.ty\n")
	for i, f := range figli {
		fmt.Fprintf(&b, "%d %s PEZZO ACME 1\n", i+1, f)
	}
	if speculare != "" {
		b.WriteString("SPECULARE DI " + speculare + "\n")
	}
	b.WriteString("Rev Mod. N. Descrizione modifica\n00 ACME-P003 Emissione\n")
	b.WriteString("Family: Part Nr: Descrizione:\n-- " + codice + " STAFFA ASSIEME ACME")
	tp := testoPDF(b.String(), "", worker.MetadatiPDF{})
	tp = conCampoCodice(tp, worker.CampoCodice, "Part Nr:", codice)
	tp.Cartiglio = append(tp.Cartiglio, worker.CampoCartiglio{Etichetta: worker.CampoTitolo, Letta: "Descrizione:", Valore: "STAFFA ASSIEME ACME",
		Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{700, 575, 800, 585}})
	return tp
}

// figliAssieme sono le dodici righe dell'elenco particolari del disegno d'assieme di prova: 7120020…7120031.
func figliAssieme() []string {
	var out []string
	for i := 0; i < 12; i++ {
		out = append(out, fmt.Sprint(7120020+i))
	}
	return out
}

// TestIlPdfDAssiemeHaIlCodiceDelCartiglio (giro 4, fase 4.6: il bloccante dello scenario del 28/09, «il PDF d'assieme
// prende il codice del primo figlio»). Il disegno d'assieme ACME ha dodici righe nell'elenco particolari dentro la
// zona del cartiglio, «SPECULARE DI 7120003» e il codice vero, 7120002, nel campo «Part Nr:». Prima: tredici letture
// del cartiglio a 85 nell'ordine del file, vinceva la prima riga (7120020, un figlio), e il taglio a MaxEvidenze
// toglieva il codice vero e la lettura del nome. Adesso:
//   - la valutazione ha il codice dell'assieme, 7120002, a 85 dal cartiglio, e nessuna evidenza dei figli ne' dello
//     speculare; con il nome che lo dice le fonti sono concordi; con il nome di un figlio sono discordi, e la colonna
//     resta il nome (non si corregge);
//   - le letture verso il flusso sono il solo codice del campo; i figli non sono letture ne' chiavi dell'indice (sono
//     la distinta del disegno, fase 4.12); lo speculare e' la nota «speculare di 7120003».
//
// La controprova (a mano): con soloDelCodice che lascia passare ogni codice della zona, i figli tornano fra le
// letture e la valutazione ha 7120020.
func TestIlPdfDAssiemeHaIlCodiceDelCartiglio(t *testing.T) {
	m := motoreACME(t)
	figli := figliAssieme()
	fatti := fattiDisegno(t, testoAssieme("7120002", "7120003", figli))

	v := valutaDisegno(t, "tavola assieme.pdf", fatti)
	controllaDim(t, "assieme", "codice", v.Codice, dimAttesa{"7120002", 85, "pdf_testo_famiglia", StatoUnica})
	if r := v.Riepilogo(); r.Codice != "7120002" || r.Confidenza != 85 || r.Fonte != "cartiglio" {
		t.Errorf("la colonna del disegno d'assieme: %+v", r)
	}
	for _, e := range v.Codice.Evidenze {
		if e.Valore != "7120002" || e.Indizio != "" {
			t.Errorf("un figlio dell'elenco (o lo speculare) e' una lettura del codice del disegno: %+v", e)
		}
	}
	controllaDim(t, "assieme con il nome", "codice", valutaDisegno(t, "7120002.pdf", fatti).Codice,
		dimAttesa{"7120002", 85, "pdf_testo_famiglia", StatoConcorde})
	conFiglio := valutaDisegno(t, "7120020.pdf", fatti)
	controllaDim(t, "assieme con il nome di un figlio", "codice", conFiglio.Codice, dimAttesa{"7120002", 85, "pdf_testo_famiglia", StatoDiscorde})
	if r := conFiglio.Riepilogo(); r.Codice != "7120020" || r.Fonte != "nome_file" {
		t.Errorf("con le fonti discordi la colonna resta il nome: %+v", r)
	}

	l := LettureDelPDF(m, fatti, "tavola assieme.pdf")
	var letti []string
	for _, x := range l.Letture {
		letti = append(letti, x.Fonte+":"+x.Codice+":"+x.Etichetta)
	}
	if strings.Join(letti, " ") != FonteTestoCartiglio+":7120002:"+worker.CampoCodice {
		t.Errorf("le letture del disegno d'assieme: %v", letti)
	}
	for _, f := range append(figli, "7120003") {
		if slices.Contains(l.Chiavi(), f) {
			t.Errorf("%s (una riga dell'elenco, o lo speculare) e' una chiave dell'indice: %v", f, l.Chiavi())
		}
	}
	if strings.Join(l.Speculari, " ") != "7120003" || len(l.Simili) != 0 {
		t.Errorf("lo speculare e' una nota: speculari %v, simili %v", l.Speculari, l.Simili)
	}
	if e := EvidenzeTestoPDF(m, testoAssieme("7120002", "7120003", figli)); len(e.Speculari) != 1 || e.Speculari[0].Etichetta != EtichettaSpeculare ||
		NotaSpeculare(e.Speculari[0].Codice) != "speculare di 7120003" {
		t.Errorf("la lettura dello speculare: %+v", e.Speculari)
	}
}

// TestIlParticolareSimileDalCampoNonEUnCodice (giro 4, fase 4.6; risposta 4 del 29/09; domanda 3 del giro 4, A finche'
// l'utente non risponde): dalla sottoversione 2 del testo il worker riporta il campo del particolare simile
// (worker.CampoParticolareSimile) con il valore grezzo. Il server lo legge con la regola dei frammenti della 4.2: il
// valore e' la nota «simile a 7120012», mai un codice del cartiglio, del testo o dell'OCR, mai una chiave
// dell'indice, mai un'evidenza della valutazione. Anche nel cartiglio a tabella (le etichette su una riga, i valori
// sotto: nel testo 7120012 non sta accanto alla sua etichetta), con il campo fuori dalla zona del cartiglio, letto
// dall'OCR. Il codice del file resta quello del suo campo («PART. N°» 7120001A1). Il segnaposto del modello nel campo
// (da un worker che non l'avesse riconosciuto), anche con il codice della colonna accanto, non da' niente.
//
// La controprova (a mano): con leggiCampo che tratta il campo del simile come gli altri campi, e con campoCon che
// non lo guarda, 7120012 torna fra le letture e le chiavi.
func TestIlParticolareSimileDalCampoNonEUnCodice(t *testing.T) {
	m := motoreACME(t)
	simile := func(tp worker.TestoPDF, valore, zona, fonte string) worker.TestoPDF {
		tp.Cartiglio = append(append([]worker.CampoCartiglio(nil), tp.Cartiglio...), worker.CampoCartiglio{Etichetta: worker.CampoParticolareSimile,
			Letta: "PARTICOLARE SIMILE / SIMILAR PART", Valore: valore, Pagina: 1, Zona: zona, Fonte: fonte, Riquadro: []float64{560, 560, 600, 572}})
		return tp
	}
	partN := func(tp worker.TestoPDF) worker.TestoPDF {
		return conCampoCodice(tp, worker.CampoCodice, "PART. N°", "7120001A1")
	}
	ocr := conOCR(partN(testoPDF("SCALA 1:1", "", worker.MetadatiPDF{})), 1, worker.ZonaBassoDestra, "SIMILAR PART\n7120012", 61)
	casi := []struct {
		nome   string
		tp     worker.TestoPDF
		simile string
	}{
		{"il cartiglio a tabella", simile(partN(testoPDF("PARTICOLARE SIMILE / SIMILAR PART   PART. N°\n7120012   7120001A1", "", worker.MetadatiPDF{})),
			"7120012", worker.ZonaBassoDestra, worker.FonteTestoNativo), "7120012"},
		{"il solo campo", simile(partN(testoPDF("SCALA 1:1", "", worker.MetadatiPDF{})), "7120012", worker.ZonaBassoDestra, worker.FonteTestoNativo), "7120012"},
		{"fuori dalla zona del cartiglio", simile(partN(testoPDF("SCALA 1:1", "SIMILAR PART\nvedi 7120012", worker.MetadatiPDF{})),
			"7120012", worker.ZonaPagina, worker.FonteTestoNativo), "7120012"},
		{"letto dall'OCR", simile(ocr, "7120012", worker.ZonaBassoDestra, worker.FonteTestoOCR), "7120012"},
		{"il segnaposto", simile(partN(testoPDF("SCALA 1:1", "", worker.MetadatiPDF{})), "Inserire codice particolare simile",
			worker.ZonaBassoDestra, worker.FonteTestoNativo), ""},
		{"il segnaposto con la colonna accanto", simile(partN(testoPDF("Inserire codice particolare simile   7120001A1", "", worker.MetadatiPDF{})),
			"Inserire codice particolare simile   7120001A1", worker.ZonaBassoDestra, worker.FonteTestoNativo), ""},
	}
	for _, c := range casi {
		fatti := fattiDisegno(t, c.tp)
		l := LettureDelPDF(m, fatti, "tavola.pdf")
		if strings.Join(l.Simili, " ") != c.simile {
			t.Errorf("%s: simili %v, atteso «%s»", c.nome, l.Simili, c.simile)
		}
		var cartiglio []string
		for _, x := range l.Letture {
			if x.Codice == "7120012" {
				t.Errorf("%s: il particolare simile e' una lettura: %+v", c.nome, x)
			}
			if x.Fonte == FonteTestoCartiglio {
				cartiglio = append(cartiglio, x.Codice+":"+x.Etichetta)
			}
		}
		if strings.Join(cartiglio, " ") != "7120001:"+worker.CampoCodice+" 7120001A1:"+worker.CampoCodice {
			t.Errorf("%s: il codice del file resta quello del suo campo: %v", c.nome, cartiglio)
		}
		if slices.Contains(l.Chiavi(), "7120012") {
			t.Errorf("%s: il particolare simile e' una chiave: %v", c.nome, l.Chiavi())
		}
		e := EvidenzeTestoPDF(m, c.tp)
		if c.simile != "" && (len(e.Simili) != 1 || e.Simili[0].Etichetta != EtichettaSimile || NotaSimile(e.Simili[0].Codice) != "simile a 7120012") {
			t.Errorf("%s: la lettura del particolare simile: %+v", c.nome, e.Simili)
		}
		v := valutaDisegno(t, "tavola.pdf", fatti)
		if v.Codice.Valore != "7120001" {
			t.Errorf("%s: il codice della valutazione: %+v", c.nome, v.Codice)
		}
		for _, x := range v.Codice.Evidenze {
			if x.Valore == "7120012" || x.Indizio == "7120012" {
				t.Errorf("%s: il particolare simile e' un'evidenza della valutazione: %+v", c.nome, x)
			}
		}
	}
}

// TestLoSpeculareEUnaNota (giro 4, fase 4.6; scenario del 28/09): «SPECULARE DI X», «COMPONENTE SPECCHIATO DI X»,
// «MIRROR OF X» dicono di quale pezzo questo e' il gemello speculare. X non e' il codice del file e non e' una
// seconda lettura: e' la nota «speculare di X», nella zona del cartiglio, nel resto della pagina (dove prima era
// una chiave dell'indice, e nel flusso il candidato diventava l'altro pezzo), dentro il valore di un campo, con il
// codice sulla riga sotto. Il codice del file resta quello del suo campo. Una frase senza un codice non da' niente.
//
// La controprova (a mano): senza separaSpeculari il caso «nel resto della pagina» ha 7120011 fra le letture e le
// chiavi, e nessuno ha la nota.
func TestLoSpeculareEUnaNota(t *testing.T) {
	m := motoreACME(t)
	campo := func(tp worker.TestoPDF) worker.TestoPDF {
		return conCampoCodice(tp, worker.CampoNumeroDisegno, "DISEGNO N.", "7120010")
	}
	titolo := campo(testoPDF("DISEGNO N. 7120010", "", worker.MetadatiPDF{}))
	titolo.Cartiglio = append(titolo.Cartiglio, worker.CampoCartiglio{Etichetta: worker.CampoTitolo, Letta: "DENOMINAZIONE / NAME",
		Valore: "STAFFA DX SPECULARE DI 7120011", Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo})
	casi := []struct {
		nome      string
		tp        worker.TestoPDF
		speculare string
	}{
		{"nella zona del cartiglio", campo(testoPDF("12.5 COMPONENTE SPECCHIATO DI 7120011\nDISEGNO N. 7120010", "", worker.MetadatiPDF{})), "7120011"},
		{"nel resto della pagina", campo(testoPDF("DISEGNO N. 7120010", "NOTA: SPECULARE DI 7120011", worker.MetadatiPDF{})), "7120011"},
		{"in inglese", campo(testoPDF("DISEGNO N. 7120010", "MIRROR OF 7120011", worker.MetadatiPDF{})), "7120011"},
		{"a capo", campo(testoPDF("DISEGNO N. 7120010", "SPECULARE DI\n7120011", worker.MetadatiPDF{})), "7120011"},
		{"nel valore di un campo", titolo, "7120011"},
		{"senza un codice", campo(testoPDF("DISEGNO N. 7120010", "SPECULARE DI STAFFA SX 7120011", worker.MetadatiPDF{})), ""},
	}
	for _, c := range casi {
		fatti := fattiDisegno(t, c.tp)
		l := LettureDelPDF(m, fatti, "tavola.pdf")
		if strings.Join(l.Speculari, " ") != c.speculare {
			t.Errorf("%s: speculari %v, atteso «%s»", c.nome, l.Speculari, c.speculare)
		}
		for _, x := range l.Letture {
			if x.Codice == "7120011" && c.speculare != "" {
				t.Errorf("%s: lo speculare e' una lettura: %+v", c.nome, x)
			}
		}
		if c.speculare != "" && slices.Contains(l.Chiavi(), "7120011") {
			t.Errorf("%s: lo speculare e' una chiave: %v", c.nome, l.Chiavi())
		}
		if len(l.Letture) == 0 || l.Letture[0].Fonte != FonteTestoCartiglio || l.Letture[0].Codice != "7120010" {
			t.Errorf("%s: il codice del file resta quello del suo campo: %+v", c.nome, l.Letture)
		}
		v := valutaDisegno(t, "tavola.pdf", fatti)
		controllaDim(t, c.nome, "codice", v.Codice, dimAttesa{"7120010", 85, "pdf_testo_famiglia", StatoUnica})
	}
	if NotaSpeculare("7120011") != "speculare di 7120011" {
		t.Errorf("la nota: %q", NotaSpeculare("7120011"))
	}
}

// TestIlTaglioNonToglieIlNome (giro 4, fase 4.6; A5.14.2): una dimensione tiene al piu' MaxEvidenze evidenze, le piu'
// forti, e comunque le letture del nome del file (taglia). Prima il taglio era soltanto per score: con dieci letture a
// 85 di valori diversi (le righe dell'elenco particolari prese per il cartiglio) la lettura del nome (70) usciva, e
// con lei il ripiego della colonna sul nome quando le fonti discordano: la colonna diventava la prima riga. Adesso
// la lettura del nome prende il posto della piu' debole delle altre, l'ordine resta per score, e la colonna e' il
// nome. Lo stesso per la rev del nome. Sotto il tetto non cambia niente.
//
// La controprova (a mano): con il taglio di prima (ord[:MaxEvidenze]) la lettura del nome manca e la colonna e' 7120020.
func TestIlTaglioNonToglieIlNome(t *testing.T) {
	var cod, rev []Evidenza
	for i := 0; i < 10; i++ {
		cod = append(cod, evidenza("pdf_testo_famiglia", fmt.Sprint(7120020+i), "riga dell'elenco"))
		rev = append(rev, evidenza("rev_step_nodo", fmt.Sprint(i), "rev"))
	}
	cod = append(cod, evidenza("nome_codice_famiglia", "7120002", "7120002_1.pdf"), evidenza("nome_contiene_codice", "", "7120099"))
	rev = append(rev, evidenza("rev_suffisso_nome", "1", "_1"))
	d := Componi(cod)
	if len(d.Evidenze) != MaxEvidenze {
		t.Fatalf("le evidenze tenute: %d, al piu' %d", len(d.Evidenze), MaxEvidenze)
	}
	nome, ok := letturaDelNome(d, prefissiNomeCodice)
	if !ok || nome.Valore != "7120002" {
		t.Errorf("la lettura del nome e' uscita dalle evidenze: %s", mustJSON(t, d.Evidenze))
	}
	if d.Evidenze[len(d.Evidenze)-1].Regola != "nome_codice_famiglia" || d.Evidenze[0].Valore != "7120020" || d.Stato != StatoDiscorde {
		t.Errorf("l'ordine resta per score, il valore e' la lettura piu' forte, le fonti discordano: %s %s", d.Stato, mustJSON(t, d.Evidenze))
	}
	for _, e := range d.Evidenze {
		if e.Regola == "nome_contiene_codice" {
			t.Errorf("una lettura del nome senza valore non ha il posto garantito: %+v", e)
		}
	}
	if r := (Valutazione{Tipo: Componi([]Evidenza{evidenza("pdf_termini_cartiglio", "disegno_2d", "")}), Codice: d, Rev: Componi(rev)}).Riepilogo(); r.Codice != "7120002" ||
		r.Confidenza != 70 || r.Rev != "1" {
		t.Errorf("con le fonti discordi la colonna resta il nome, con la sua rev: %+v", r)
	}
	if dr := Componi(rev); len(dr.Evidenze) != MaxEvidenze || dr.Evidenze[len(dr.Evidenze)-1].Regola != "rev_suffisso_nome" {
		t.Errorf("la rev del nome resta fra le evidenze: %s", mustJSON(t, dr.Evidenze))
	}
	// sotto il tetto niente cambia
	poche := []Evidenza{evidenza("pdf_testo_famiglia", "7120020", ""), evidenza("nome_codice_famiglia", "7120002", "")}
	if d := Componi(poche); len(d.Evidenze) != 2 || d.Evidenze[0].Valore != "7120020" || d.Evidenze[1].Valore != "7120002" {
		t.Errorf("sotto il tetto: %s", mustJSON(t, d.Evidenze))
	}
}

// TestLaSottoversioneVecchiaSiRilegge (giro 4, fase 4.6; domanda 21 del giro 4 senza risposta: niente versione nuova
// dell'analizzatore): i fatti con il testo di una sottoversione di prima di quella di oggi (worker.VersioneTestoPDF) si
// leggono, e si rileggono con «Rianalizza» (TestoPDFDaRileggere); quelli della sottoversione di oggi no. Il testo non
// letto si rilegge come prima; un PDF che non si apre no.
func TestLaSottoversioneVecchiaSiRilegge(t *testing.T) {
	testo := func(versione int, estraibile bool) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"testo_pdf": {"versione": %d, "estraibile": %v, "frammenti": []}}`, versione, estraibile))
	}
	if worker.VersioneTestoPDF != 2 {
		t.Errorf("la sottoversione del testo della fase 4.6 e' la 2: %d", worker.VersioneTestoPDF)
	}
	casi := []struct {
		nome    string
		fatti   json.RawMessage
		stato   string
		rileggi bool
	}{
		{"sottoversione 1, letto", testo(1, true), TestoLetto, true},
		{"sottoversione 1, senza testo", testo(1, false), TestoAssente, true},
		{"sottoversione di oggi", testo(worker.VersioneTestoPDF, true), TestoLetto, false},
		{"sottoversione di oggi, senza testo", testo(worker.VersioneTestoPDF, false), TestoAssente, false},
		{"una sottoversione piu' nuova", testo(worker.VersioneTestoPDF+1, true), TestoLetto, false},
		{"testo non letto", json.RawMessage(`{"cartiglio": true}`), TestoNonLetto, true},
		{"illeggibile", json.RawMessage(`{"errore_pdf": "broken"}`), TestoIlleggibile, false},
	}
	for _, c := range casi {
		if s := StatoDelTestoPDF(c.fatti); s != c.stato {
			t.Errorf("%s: stato %s, atteso %s", c.nome, s, c.stato)
		}
		if r := TestoPDFDaRileggere(c.fatti); r != c.rileggi {
			t.Errorf("%s: da rileggere %v, atteso %v", c.nome, r, c.rileggi)
		}
	}
}

// testoDiPrimaPDF e' il testo come l'avrebbe riportato il worker della sottoversione 1, prima della fase 4.6.
func testoDiPrimaPDF(tp worker.TestoPDF) worker.TestoPDF {
	tp.Versione = 1
	return tp
}

// TestIlCartiglioDiUnTestoDiPrimaEUnIndizio (giro 4, fase 4.6r): i PDF gia' letti dal worker installato sulle
// postazioni hanno il testo della sottoversione 1. Letti con la regola della 4.6 (nella zona del cartiglio vota il
// solo campo del codice), il disegno d'assieme senza codice nel nome passava da «discorde» (ogni codice della zona a
// 85) a «unica 85» sul primo figlio, perche' il campo del codice del worker di prima era l'intestazione dell'elenco
// («Part Number») con il valore della riga sotto: il bug 3 della Distinta, finche' nessuno rianalizzava. E un
// cartiglio con un'etichetta che il worker di prima non conosceva («PART. N°») perdeva la lettura. Adesso, finche' il
// testo e' della sottoversione 1:
//   - le letture del cartiglio sono tutti i codici della zona (meno le note), segnate DaRileggere: restano chiavi di
//     ricerca; lo stato e' «letto» e la frase «testo letto con il worker di prima: da rianalizzare»;
//   - nella valutazione sono indizi senza voto (RegolaCartiglioDiPrima), con la frase: il codice e' quello del nome,
//     o nessuno, e il cartiglio di prima non fa ne' «concorde» ne' «discorde»;
//   - lo speculare resta una nota;
//
// e il testo di oggi (dopo «Rianalizza») da' il codice dell'assieme, 7120002, a 85.
//
// Le controprove (a mano): con soloDelCodice che tratta il testo di prima come quello di oggi, la valutazione e'
// «unica 85» su 7120020 e il cartiglio con «PART. N°» non ha letture ne' chiavi; con il cartiglio di prima che vota
// in evidenzeCodiceDelTesto, la valutazione del disegno d'assieme ha un valore dal cartiglio.
func TestIlCartiglioDiUnTestoDiPrimaEUnIndizio(t *testing.T) {
	m := motoreACME(t)
	figli := figliAssieme()
	// il disegno d'assieme come lo leggeva il worker di prima: il campo del codice e' l'intestazione dell'elenco, con
	// la prima riga come valore; «Part Nr:» non lo conosceva
	vecchio := testoAssieme("7120002", "7120003", figli)
	vecchio.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "Part Number", Valore: "7120020", Pagina: 1,
		Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{595, 311, 635, 321}}}
	fatti := fattiDisegno(t, testoDiPrimaPDF(vecchio))

	v := valutaDisegno(t, "tavola assieme.pdf", fatti)
	controllaDim(t, "assieme di prima", "codice", v.Codice, dimAttesa{"", 0, RegolaCartiglioDiPrima, StatoNessuna})
	indizi := map[string]bool{}
	for _, e := range v.Codice.Evidenze {
		if e.Valore != "" || e.Regola != RegolaCartiglioDiPrima || e.Dove != DoveBassoDestra+", "+FraseTestoDiPrima || e.Famiglia != "ACME 712" {
			t.Errorf("il cartiglio di prima vota, o non dice perche' non vota: %+v", e)
		}
		indizi[e.Indizio] = true
	}
	if !indizi["7120020"] || indizi["7120003"] {
		t.Errorf("gli indizi del cartiglio di prima: %v", indizi)
	}
	if Indizi[RegolaCartiglioDiPrima].Parole == "" || Indizi[RegolaCartiglioDiPrima].Score != 0 {
		t.Errorf("l'indizio del cartiglio di prima ha le sue parole e nessuno score: %+v", Indizi[RegolaCartiglioDiPrima])
	}
	// con il nome dell'assieme, o di un figlio: il codice e' quello del nome, da una sola fonte
	controllaDim(t, "assieme di prima con il nome", "codice", valutaDisegno(t, "7120002.pdf", fatti).Codice,
		dimAttesa{"7120002", 70, "nome_codice_famiglia", StatoUnica})
	controllaDim(t, "assieme di prima con il nome di un figlio", "codice", valutaDisegno(t, "7120021.pdf", fatti).Codice,
		dimAttesa{"7120021", 70, "nome_codice_famiglia", StatoUnica})

	l := LettureDelPDF(m, fatti, "tavola assieme.pdf")
	if l.Stato != TestoLetto || !l.DaRileggere || l.Frase != FraseTestoDiPrima {
		t.Errorf("il testo di prima: stato %s, da rileggere %v, frase %q", l.Stato, l.DaRileggere, l.Frase)
	}
	var cartiglio []string
	for _, x := range l.Letture {
		if x.Fonte != FonteTestoCartiglio {
			continue
		}
		if !x.DaRileggere {
			t.Errorf("una lettura del cartiglio di prima non e' da rileggere: %+v", x)
		}
		cartiglio = append(cartiglio, x.Codice)
	}
	for _, c := range append(append([]string(nil), figli...), "7120002") {
		if !slices.Contains(cartiglio, c) || !slices.Contains(l.Chiavi(), c) {
			t.Errorf("%s del cartiglio di prima non resta una lettura e una chiave: %v, chiavi %v", c, cartiglio, l.Chiavi())
		}
	}
	if slices.Contains(cartiglio, "7120003") || strings.Join(l.Speculari, " ") != "7120003" {
		t.Errorf("lo speculare resta una nota: %v, speculari %v", cartiglio, l.Speculari)
	}

	// un cartiglio con un'etichetta che il worker di prima non conosceva: la lettura resta, da rileggere
	ignota := fattiDisegno(t, testoDiPrimaPDF(testoPDF("PART. N° 7120001A1\nSCALA 1:1", "", worker.MetadatiPDF{})))
	l = LettureDelPDF(m, ignota, "tavola.pdf")
	if len(l.Letture) == 0 || l.Letture[0].Codice != "7120001" || !l.Letture[0].DaRileggere || !slices.Contains(l.Chiavi(), "7120001") {
		t.Errorf("il cartiglio di prima senza il campo del codice: %+v", l.Letture)
	}
	vi := valutaDisegno(t, "tavola.pdf", ignota)
	if e, ok := evidenzaDi(vi.Codice, RegolaCartiglioDiPrima); vi.Codice.Valore != "" || !ok || e.Indizio != "7120001" {
		t.Errorf("il cartiglio di prima senza il campo del codice nella valutazione: %+v", vi.Codice)
	}

	// il testo di oggi, dopo «Rianalizza»: il campo del codice, a 85, e niente da rileggere
	oggi := fattiDisegno(t, testoAssieme("7120002", "7120003", figli))
	controllaDim(t, "assieme di oggi", "codice", valutaDisegno(t, "tavola assieme.pdf", oggi).Codice,
		dimAttesa{"7120002", 85, "pdf_testo_famiglia", StatoUnica})
	if l := LettureDelPDF(m, oggi, "tavola assieme.pdf"); l.DaRileggere || l.Frase != "" || len(l.Letture) != 1 || l.Letture[0].DaRileggere {
		t.Errorf("il testo di oggi: %+v", l)
	}
}
