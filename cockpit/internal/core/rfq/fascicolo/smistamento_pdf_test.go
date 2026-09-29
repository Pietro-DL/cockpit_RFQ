package fascicolo

// L1 — Giro 4, fase 4.2: F9 → F8, una porta sola per il PDF (precisazione dell'utente del 27/09 sera: F9 produce
// fatti, il core li normalizza, F8 consuma evidenze normalizzate; addendum A5.13.3 passi 4-5, prove 163 e 164;
// risposta 4 del 29/09 sul «particolare simile»). Le evidenze del flusso sono le letture normalizzate della
// classificazione (LettureDelPDF), con lo stato del testo; il disegno del prodotto senza STEP e' un'ancora
// piatta, mai preselezionata; un PDF senza testo non ancora e lo dice; il particolare simile e' una nota. Le prove
// con il database stanno in smistamento_db_test.go e in proposte_db_test.go.

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// motoreACME712 sono le regole finte di ACME: la famiglia 712 dei codici di prova.
func motoreACME712(t *testing.T) *classificazione.Motore {
	t.Helper()
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	if !m.HaFamiglie() {
		t.Fatal("la famiglia di prova non e' entrata nel motore")
	}
	return m
}

// testoFinto e' il fatto `testo_pdf` di un PDF finto: un frammento nativo in basso a destra della pagina 1 (il
// probabile cartiglio) e uno nel resto della pagina, con i metadati dati.
func testoFinto(bassoDestra, pagina string, meta worker.MetadatiPDF) worker.TestoPDF {
	t := worker.TestoPDF{Versione: 1, Pagine: 1, PagineLette: 1, Metadati: meta, FormatoPagina1: []float64{842, 595},
		OCR:    worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Limiti: worker.LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384}}
	if bassoDestra != "" {
		t.Frammenti = append(t.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo,
			Testo: bassoDestra, Riquadro: []float64{560, 490, 724, 543}})
		t.Caratteri += len(bassoDestra)
	}
	if pagina != "" {
		t.Frammenti = append(t.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: worker.ZonaPagina, Fonte: worker.FonteTestoNativo,
			Testo: pagina, Riquadro: []float64{40, 50, 200, 63}})
		t.Caratteri += len(pagina)
	}
	t.Estraibile = t.Caratteri > 0
	return t
}

// fattiPDF sono i fatti dell'analizzatore 4 di un disegno con quel testo.
func fattiPDF(t *testing.T, tp worker.TestoPDF) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"}, "testo_pdf": tp})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// letto da' al PDF della scena lo stato del testo e le evidenze che LeggiStatoFlusso gli darebbe da quei fatti:
// le letture della classificazione per il SUO nome, normalizzate da evidenzeDalleLetture.
func (sc *scena) letto(nome string, fatti json.RawMessage) {
	f := sc.f(nome)
	f.TestoPDF, f.EvidenzePDF = evidenzeDalleLetture(classificazione.LettureDelPDF(sc.s.Motore, fatti, nome))
}

// disegnoLetto e' un disegno della scena letto dal worker con quei fatti: la valutazione e il testo vengono
// dagli stessi fatti, come nell'analisi.
func (sc *scena) disegnoLetto(nome string, fatti json.RawMessage) {
	sc.file(nome, shaDel(nome), sc.valuta(nome, &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, string(fatti), nil))
	sc.letto(nome, fatti)
}

func evidenzeIn(d Destinazione) string { return strings.Join(d.Evidenze, " | ") }

// Giro 4, fase 4.2: le evidenze del PDF che il flusso consuma sono le letture normalizzate della
// classificazione, una per una, nello stesso ordine (cartiglio, metadati, resto del testo, OCR), con le fonti
// della classificazione, l'origine (famiglia o generico), l'etichetta del campo, il riquadro, la dipendenza dal
// nome del file e l'indizio dell'OCR; lo stato del testo con la sua frase e l'OCR. Il disegno «7120001_1.pdf» ha
// il cartiglio con 7120001 (conferma il nome, non ne dipende) e un numero generico, il titolo che ripete il nome,
// una nota con 7120012 nel resto della pagina e l'OCR con 7120010; poi i fatti senza testo, un PDF muto con l'OCR
// assente, un PDF che non si apre. La controprova (a mano): con l'OCR non segnato come indizio, o con l'origine
// persa, la prova fallisce.
func TestLeEvidenzeDelPdfSonoLeLettureNormalizzate(t *testing.T) {
	m := motoreACME712(t)
	if FontePDFCartiglio != classificazione.FonteTestoCartiglio || FontePDFTesto != classificazione.FonteTestoPagina ||
		FontePDFMetadati != classificazione.FonteMetadatiPDF || FontePDFOCR != classificazione.FonteOCR {
		t.Fatal("le fonti del flusso sono le costanti della classificazione")
	}
	tp := testoFinto("DISEGNO N. 7120001\nSCALA 1:2 ORDINE 4500123", "NOTA: vedi 7120012", worker.MetadatiPDF{Titolo: "7120001_1.pdf"})
	tp.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoNumeroDisegno, Letta: "DISEGNO N.", Valore: "7120001", Pagina: 1,
		Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{620, 530, 655, 543}}}
	conf := 71.0
	tp.Frammenti = append(tp.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoOCR,
		Testo: "7120010", Riquadro: []float64{600, 500, 700, 512}, Confidenza: &conf})
	tp.OCR = worker.OCRPDF{Stato: worker.OCREseguito, Motore: "finto"}
	fatti := fattiPDF(t, tp)

	l := classificazione.LettureDelPDF(m, fatti, "7120001_1.pdf")
	st, ev := evidenzeDalleLetture(l)
	if st.Stato != classificazione.TestoLetto || st.Frase != "" || st.OCR != worker.OCREseguito || !st.Letto() {
		t.Errorf("lo stato del testo letto: %+v", st)
	}
	if len(ev) != len(l.Letture) || len(ev) < 4 {
		t.Fatalf("una evidenza per lettura: %d evidenze, %d letture (%+v)", len(ev), len(l.Letture), ev)
	}
	for i, x := range l.Letture {
		e := ev[i]
		origine := OrigineGenerico
		if x.Origine == "famiglia" {
			origine = OrigineFamiglia
		}
		if e.Fonte != x.Fonte || e.Codice != strings.ToUpper(x.Codice) || e.Pagina != x.Pagina || e.Origine != origine ||
			e.Famiglia != x.Famiglia || e.Etichetta != x.Etichetta || e.DipendeDaNome != x.DipendeDaNome || e.Indizio != x.Indizio() {
			t.Errorf("l'evidenza %d non e' la lettura normalizzata: %+v da %+v", i, e, x)
		}
		if (e.Posizione == nil) != (len(x.Riquadro) != 4) || (e.Posizione != nil && e.Posizione.X0 != x.Riquadro[0]) {
			t.Errorf("l'evidenza %d: il riquadro %+v da %v", i, e.Posizione, x.Riquadro)
		}
	}
	trova := func(fonte, codice string) (EvidenzaContenutoPDF, bool) {
		for _, e := range ev {
			if e.Fonte == fonte && e.Codice == codice {
				return e, true
			}
		}
		return EvidenzaContenutoPDF{}, false
	}
	if e, ok := trova(FontePDFCartiglio, "7120001"); !ok || e.Origine != OrigineFamiglia || e.DipendeDaNome || e.Indizio ||
		e.Etichetta != worker.CampoNumeroDisegno || !e.DalCartiglio() {
		t.Errorf("il cartiglio di famiglia che conferma il nome: %+v %v", e, ok)
	}
	if e, ok := trova(FontePDFCartiglio, "4500123"); !ok || e.Origine != OrigineGenerico {
		t.Errorf("il numero generico dove sta il cartiglio: %+v %v", e, ok)
	}
	if e, ok := trova(FontePDFMetadati, "7120001"); !ok || !e.DipendeDaNome {
		t.Errorf("il titolo che ripete il nome dipende dal nome: %+v %v", e, ok)
	}
	if e, ok := trova(FontePDFTesto, "7120012"); !ok || e.DipendeDaNome || e.Indizio {
		t.Errorf("la nota nel resto della pagina: %+v %v", e, ok)
	}
	if e, ok := trova(FontePDFOCR, "7120010"); !ok || !e.Indizio || e.DalCartiglio() {
		t.Errorf("l'OCR e' un indizio, mai il cartiglio: %+v %v", e, ok)
	}

	// senza il testo: lo stato e la frase della classificazione, nessuna evidenza
	muto := testoFinto("", "", worker.MetadatiPDF{})
	muto.OCR = worker.OCRPDF{Stato: worker.OCRNonDisponibile, Motivo: "motore assente"}
	for _, c := range []struct {
		nome  string
		fatti json.RawMessage
		stato string
		frase string
	}{
		{"senza testo_pdf (analizzatore di prima)", json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`), classificazione.TestoNonLetto, "da rianalizzare"},
		{"senza testo nel file", fattiPDF(t, muto), classificazione.TestoAssente, "serve l'OCR, che sul worker non c'e'"},
		{"illeggibile", json.RawMessage(`{"errore_pdf": "cannot open broken document"}`), classificazione.TestoIlleggibile, "non si apre"},
	} {
		st, ev := evidenzeDalleLetture(classificazione.LettureDelPDF(m, c.fatti, "7120001_1.pdf"))
		if st.Stato != c.stato || st.Letto() || !strings.Contains(st.Frase, c.frase) || len(ev) != 0 ||
			st.Frase != classificazione.FraseTestoPDF(c.stato, st.OCR) {
			t.Errorf("%s: %+v, %d evidenze", c.nome, st, len(ev))
		}
	}
}

// Prova 163 (A5.13.3, passo 4: A-P1, A-P2 con solo_metadati, A-P3; giro 4, fase 4.2): senza STEP il disegno del
// prodotto, con il codice del prodotto scritto dentro, e' un'ancora piatta. Il prodotto 7120002 non ha STEP; il
// cartiglio di «7120002.pdf» dice 7120002 (A-P1, 80 + 10 con il nome), il titolo di «tavola.pdf» lo dice solo
// nei metadati (A-P2, 50, solo_metadati), il testo di «vista.pdf» lo cita fuori dal cartiglio (A-P3, 30, da
// confermare). Ognuno ha il candidato verso il prodotto, e nessuno e' preselezionato (scelta 2); il capitolato
// che nomina il prodotto non ancora; il disegno del figlio 7120020 resta «manca un riferimento strutturale», con
// il motivo dell'ancora piatta: l'ancora da' il prodotto, non i figli (Domanda 4 = B), e il testo del PDF non
// entra nell'indice. L'ancora piatta entra nella firma dell'indice. La controprova (a mano): senza il passo 4
// l'ancora torna assente e la prova fallisce.
func TestSenzaStepIlPdfDelProdottoEUnAncoraPiatta(t *testing.T) {
	costruisci := func(conTesto bool) *scena {
		sc := nuovaScena(t)
		sc.s.Motore = motoreACME712(t)
		sc.prodotto("7120002")
		if conTesto {
			sc.disegnoLetto("7120002.pdf", fattiPDF(t, testoFinto("DISEGNO N. 7120002\nSCALA 1:1", "", worker.MetadatiPDF{})))
		} else {
			sc.disegnoLetto("7120002.pdf", json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`))
		}
		sc.disegnoLetto("tavola.pdf", fattiPDF(t, testoFinto("SCALA 1:1", "", worker.MetadatiPDF{Titolo: "7120002"})))
		sc.disegnoLetto("vista.pdf", fattiPDF(t, testoFinto("SCALA 1:5", "vista dell'assieme 7120002", worker.MetadatiPDF{})))
		sc.disegnoLetto("7120020.pdf", fattiPDF(t, testoFinto("DISEGNO N. 7120020", "", worker.MetadatiPDF{})))
		cap := fattiPDF(t, testoFinto("", "REQUISITI DI FORNITURA per 7120002", worker.MetadatiPDF{}))
		sc.file("Capitolato 7120002.pdf", shaDel("capitolato"), sc.valuta("Capitolato 7120002.pdf",
			&classificazione.Esito{Tipo: "capitolato", Fonte: "cartiglio"}, string(cap), nil))
		sc.letto("Capitolato 7120002.pdf", cap)
		return sc
	}
	sc := costruisci(true)
	c := sc.calcola()
	a := c.Ancore["7120002"]
	var nomi, regole, disc []string
	var pesi []int
	for _, p := range a.Portatori {
		nomi, pesi = append(nomi, p.Nome), append(pesi, p.Peso)
		regole = append(regole, strings.Join(p.Regole, "+"))
		disc = append(disc, strings.Join(p.Discordanze, "+"))
		if p.Livello != LivelloPiatta {
			t.Errorf("%s: il portatore di un'ancora piatta e' piatto: %s", p.Nome, p.Livello)
		}
	}
	if a.Livello != LivelloPiatta || a.Ancorato() || strings.Join(nomi, " ") != "7120002.pdf tavola.pdf vista.pdf" ||
		strings.Join(regole, " ") != "A-P1+A-N1 A-P2 A-P3" || strings.Join(disc, " ") != " solo_metadati ancora_da_confermare" ||
		len(pesi) != 3 || pesi[0] != 90 || pesi[1] != 50 || pesi[2] != 30 || len(a.Discordanze) != 0 {
		t.Fatalf("l'ancora piatta: %s, ancorato %v, %v %v %v %v, %v", a.Livello, a.Ancorato(), nomi, pesi, regole, disc, a.Discordanze)
	}
	if m := strings.Join(a.Motivi, " | "); !strings.Contains(m, "il cartiglio di 7120002.pdf dice 7120002 (A-P1)") ||
		!strings.Contains(m, "non ai suoi figli") || strings.Contains(m, "Capitolato") && !strings.Contains(m, "non il disegno del prodotto") {
		t.Errorf("i motivi dell'ancora piatta: %s", m)
	}
	if _, ok := c.Indice.Voci["7120002"]; ok {
		t.Error("l'ancora piatta non porta il prodotto nell'indice come un riferimento strutturale")
	}
	if _, ok := c.Indice.Voci["7120020"]; ok {
		t.Error("il testo del disegno non porta i figli nell'indice")
	}
	casi := []struct {
		nome, riga, fonti, disc string
		contenuto               bool
	}{
		{"7120002.pdf", "1 7120002 dest_componente_contenuto", "cartiglio_pdf nome_file", "", true},
		{"tavola.pdf", "1 7120002 dest_componente_nome", "metadati_pdf", DiscSoloMetadati, false},
		{"vista.pdf", "1 7120002 dest_componente_nome", "testo_pdf", DiscAncoraDaConfermare, false},
	}
	for _, x := range casi {
		d := sc.dest(c, x.nome)
		if d.Esito != EsitoProposta || candidati(d) != x.riga || d.Preselezionabile || d.Stato != "da_scegliere" {
			t.Errorf("%s: %s «%s» preselezionabile %v stato %s", x.nome, d.Esito, candidati(d), d.Preselezionabile, d.Stato)
			continue
		}
		top := d.Candidati[0]
		if top.Bersaglio != "prodotto" || top.Ruolo != "disegno_del_prodotto" || strings.Join(top.Fonti, " ") != x.fonti ||
			top.DalContenuto != x.contenuto || !contiene(top.Sostegno, SostegnoProdotto) || strings.Join(top.Discordanze, " ") != x.disc {
			t.Errorf("%s: il candidato verso il prodotto %+v", x.nome, top)
		}
		if e := strings.Join(top.Evidenze, " | "); !strings.Contains(e, x.nome+" è il disegno del prodotto 7120002") || !strings.Contains(e, "niente è preselezionato") {
			t.Errorf("%s: le evidenze del candidato: %s", x.nome, e)
		}
		var ad *AncoraDest
		for i := range d.Ancore {
			if d.Ancore[i].Prodotto == "7120002" {
				ad = &d.Ancore[i]
			}
		}
		if ad == nil || ad.Livello != LivelloPiatta || strings.Join(ad.File, " ") != "7120002.pdf tavola.pdf vista.pdf" {
			t.Errorf("%s: l'ancora fotografata nella destinazione: %+v", x.nome, ad)
		}
	}
	figlio := sc.dest(c, "7120020.pdf")
	if figlio.Esito != EsitoSospesa || figlio.Motivo != MotivoManca || len(figlio.Candidati) != 0 ||
		strings.Join(figlio.ProdottiSenzaAncora, " ") != "7120002" || !strings.Contains(evidenzeIn(figlio), "il cartiglio di 7120002.pdf dice 7120002 (A-P1)") {
		t.Errorf("il disegno del figlio senza riferimento strutturale: %s %s «%s» %v %s", figlio.Esito, figlio.Motivo, candidati(figlio),
			figlio.ProdottiSenzaAncora, evidenzeIn(figlio))
	}
	if d := sc.dest(c, "Capitolato 7120002.pdf"); candidati(d) != "1 generale dest_generale_contenuto" {
		t.Errorf("il capitolato che nomina il prodotto non ancora e va fra i generali: %s", candidati(d))
	}

	// la firma dell'indice cambia con l'ancora piatta
	senza := costruisci(false)
	cs := senza.calcola()
	if cs.Ancore["7120002"].Portatori[0].Nome != "tavola.pdf" || cs.Indice.Firma == c.Indice.Firma {
		t.Errorf("senza il cartiglio di 7120002.pdf l'ancora e la firma cambiano: %+v, firma uguale %v", cs.Ancore["7120002"], cs.Indice.Firma == c.Indice.Firma)
	}

	// un codice della richiesta senza il suo componente: il candidato e' «Crea il prodotto», mai preselezionato
	sc = nuovaScena(t)
	sc.s.Motore = motoreACME712(t)
	sc.identificativo("7120003", true)
	sc.disegnoLetto("7120003.pdf", fattiPDF(t, testoFinto("DISEGNO N. 7120003", "", worker.MetadatiPDF{})))
	if d := sc.dest(sc.calcola(), "7120003.pdf"); candidati(d) != "1 7120003 dest_identificativo" || d.Preselezionabile ||
		d.Candidati[0].Chiave != "identificativo:7120003" {
		t.Errorf("il disegno di un codice della richiesta senza componente: %s %v %+v", candidati(d), d.Preselezionabile, d.Candidati)
	}
}

// Prova 164 (A5.13.3, passo 4; giro 4, fase 4.2): un PDF senza testo non ancora, e lo dice. Il prodotto 7120002 non
// ha STEP, e il suo disegno «7120002.pdf» non ha il testo: nel file non c'e' (curve o scansione, con l'OCR assente
// sulla postazione), i fatti sono di un analizzatore di prima (non letto, da rianalizzare), il PDF non si apre,
// l'OCR legge 7120002 (un indizio: non ancora mai), il testo ripete soltanto il nome del file. In ogni caso
// l'ancora e' assente, il motivo dice perche' con la frase dello stato del testo, e il disegno e' «manca un
// riferimento strutturale» con la stessa frase. Lo stesso quando il testo nativo c'e' (letto) e l'OCR selettivo
// legge il cartiglio raster: lo stato non ferma niente, l'indizio si' (dicePdf). Un PDF compatibile ancora in
// analisi e' un'attesa (passo 5), non una mancanza; e la firma dell'indice cambia quando il PDF da analizzare
// diventa non letto, anche se l'ancora resta assente (Ancora.testiPdf). Le controprove (a mano): senza il
// controllo dell'indizio in dicePdf, con l'OCR contato come cartiglio, il PDF col testo letto e l'OCR ancora
// (A-P1) e la prova fallisce; senza testiPdf nella firma, la firma non cambia.
func TestUnPdfSenzaTestoNonAncoraELoDice(t *testing.T) {
	muto := testoFinto("", "", worker.MetadatiPDF{})
	muto.OCR = worker.OCRPDF{Stato: worker.OCRNonDisponibile, Motivo: "motore assente"}
	conOCR := testoFinto("", "", worker.MetadatiPDF{})
	conf := 64.0
	conOCR.Frammenti = []worker.FrammentoPDF{{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoOCR, Testo: "DISEGNO N. 7120002",
		Riquadro: []float64{600, 500, 700, 512}, Confidenza: &conf}}
	conOCR.OCR = worker.OCRPDF{Stato: worker.OCREseguito, Motore: "finto"}
	ripete := testoFinto("stampato da 7120002.pdf", "", worker.MetadatiPDF{})
	// il testo nativo c'e' (letto) e il cartiglio raster lo legge l'OCR selettivo: l'OCR resta un indizio anche qui
	misto := testoFinto("SCALA 1:1", "", worker.MetadatiPDF{})
	misto.Frammenti = append(misto.Frammenti, conOCR.Frammenti...)
	misto.OCR = worker.OCRPDF{Stato: worker.OCREseguito, Motore: "finto"}
	casi := []struct {
		nome   string
		fatti  json.RawMessage
		motivo string // nel motivo dell'ancora e nelle evidenze del disegno
		ocr    string // la frase dell'indizio, se c'e'
		letto  bool   // lo stato del testo e' «letto» (l'ancora la ferma dicePdf, non lo stato)
	}{
		{"senza testo nel file", fattiPDF(t, muto), "il PDF non ha testo nel file (curve o scansione): serve l'OCR, che sul worker non c'e'", "", false},
		{"testo non letto", json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`), "testo del PDF non letto", "", false},
		{"illeggibile", json.RawMessage(`{"errore_pdf": "cannot open broken document"}`), "il PDF non si apre", "", false},
		{"solo l'OCR", fattiPDF(t, conOCR), "letto con l'OCR, solo come indizio", "l'OCR legge 7120002 (pagina 1): un indizio, non un codice del file", false},
		{"ripete il nome", fattiPDF(t, ripete), "il testo dice 7120002 solo ripetendo il nome del file", "", true},
		{"testo letto e OCR", fattiPDF(t, misto), "l'OCR legge 7120002, un indizio: non ancora", "l'OCR legge 7120002 (pagina 1): un indizio, non un codice del file", true},
	}
	for _, x := range casi {
		sc := nuovaScena(t)
		sc.s.Motore = motoreACME712(t)
		sc.prodotto("7120002")
		sc.disegnoLetto("7120002.pdf", x.fatti)
		if sc.f("7120002.pdf").TestoPDF.Letto() != x.letto {
			t.Fatalf("%s: lo stato del testo %s", x.nome, sc.f("7120002.pdf").TestoPDF.Stato)
		}
		c := sc.calcola()
		a := c.Ancore["7120002"]
		if a.Livello != LivelloAssente || len(a.Portatori) != 0 || !strings.Contains(strings.Join(a.Motivi, " | "), x.motivo) {
			t.Errorf("%s: l'ancora %s %+v, motivi %v", x.nome, a.Livello, a.Portatori, a.Motivi)
		}
		d := sc.dest(c, "7120002.pdf")
		if d.Esito != EsitoSospesa || d.Motivo != MotivoManca || len(d.Candidati) != 0 || !strings.Contains(evidenzeIn(d), x.motivo) {
			t.Errorf("%s: il disegno %s %s «%s»: %s", x.nome, d.Esito, d.Motivo, candidati(d), evidenzeIn(d))
		}
		if x.ocr != "" && !strings.Contains(evidenzeIn(d), x.ocr) {
			t.Errorf("%s: l'indizio dell'OCR si dice: %s", x.nome, evidenzeIn(d))
		}
		if x.ocr == "" && strings.Contains(evidenzeIn(d), "l'OCR legge") {
			t.Errorf("%s: un indizio che non c'e': %s", x.nome, evidenzeIn(d))
		}
	}

	// il disegno compatibile che si sta ancora analizzando: attesa, non mancanza
	sc := nuovaScena(t)
	sc.s.Motore = motoreACME712(t)
	sc.prodotto("7120002")
	sc.disegno("7120002.pdf")
	sc.f("7120002.pdf").Analisi = AnalisiInCorso
	sc.f("7120002.pdf").TestoPDF = TestoDelPDF{Stato: classificazione.TestoDaAnalizzare,
		Frase: classificazione.FraseTestoPDF(classificazione.TestoDaAnalizzare, "")}
	sc.disegno("7120020.pdf")
	c := sc.calcola()
	if a := c.Ancore["7120002"]; a.Livello != LivelloInAttesa || len(a.Portatori) != 1 || a.Portatori[0].Nome != "7120002.pdf" {
		t.Errorf("il PDF del prodotto in analisi: %+v", a)
	}
	if d := sc.dest(c, "7120020.pdf"); d.Esito != EsitoInAttesa {
		t.Errorf("il disegno del figlio aspetta il PDF del prodotto: %s %s", d.Esito, d.Motivo)
	}

	// la firma dell'indice segue lo stato del testo dei PDF guardati anche quando l'ancora non cambia
	// (Ancora.testiPdf): il PDF compatibile da analizzare, poi analizzato senza il testo (non letto), lascia
	// l'ancora assente, ma i motivi e le destinazioni che li riportano cambiano, e la firma con loro
	firma := func(stato string) (string, Ancora) {
		sc := nuovaScena(t)
		sc.s.Motore = motoreACME712(t)
		sc.prodotto("7120002")
		sc.disegno("7120002.pdf")
		sc.f("7120002.pdf").Analisi = AnalisiAssente
		if stato == classificazione.TestoNonLetto {
			sc.f("7120002.pdf").Analisi = AnalisiCorrente
		}
		sc.f("7120002.pdf").TestoPDF = TestoDelPDF{Stato: stato, Frase: classificazione.FraseTestoPDF(stato, "")}
		c := sc.calcola()
		return c.Indice.Firma, c.Ancore["7120002"]
	}
	f1, a1 := firma(classificazione.TestoDaAnalizzare)
	f2, a2 := firma(classificazione.TestoNonLetto)
	if a1.Livello != LivelloAssente || a2.Livello != LivelloAssente || len(a1.Portatori)+len(a2.Portatori) != 0 {
		t.Fatalf("la scena della firma: %s %+v, %s %+v", a1.Livello, a1.Portatori, a2.Livello, a2.Portatori)
	}
	if f1 == f2 {
		t.Errorf("il PDF del prodotto da analizzare e poi non letto: la firma dell'indice non cambia (%v → %v)", a1.Motivi, a2.Motivi)
	}
}

// Giro 4, fase 4.2 (risposta 4 del 29/09; domanda 3 del giro 4, A finche' non risponde): il «particolare simile» non
// e' mai un codice. Nel cartiglio ACME finto «PARTICOLARE SIMILE / SIMILAR PART 7120012» sta accanto a «PART. N°
// 7120001A1»: le letture danno 7120001 e la nota «simile a 7120012»; 7120012 non e' un codice del cartiglio, del
// testo, dei metadati, dell'OCR, ne' una chiave dell'indice, e nella valutazione non c'e'. Nel flusso il disegno ha
// il candidato del prodotto 7120001 (dal suo STEP autorizzato) e nessuno verso 7120012, che pure e' un figlio
// dello STEP; la destinazione porta la nota. Lo stesso con l'etichetta a capo, e dentro il valore di un campo del
// cartiglio. Il segnaposto «Inserire codice particolare simile» non da' niente: ne' una nota, ne' il codice della
// riga dopo. Nemmeno il cartiglio a tabella con il campo simile vuoto («PART. N°   PARTICOLARE SIMILE», a capo
// il codice del file): la parola sotto l'etichetta e' il valore di un'altra colonna, e 7120001 resta fra le
// letture del cartiglio, senza nota, e il disegno del prodotto 7120001 lo ancora (A-P1). La controprova (a mano):
// senza separaSimili, 7120012 torna fra i codici del cartiglio; senza aCapoDaSolo, il cartiglio a tabella perde
// 7120001 e da' la nota «simile a 7120001».
func TestIlParticolareSimileNonEUnCodice(t *testing.T) {
	m := motoreACME712(t)
	accanto := testoFinto("PARTICOLARE SIMILE / SIMILAR PART 7120012\nPART. N° 7120001A1\nSCALA 1:1", "", worker.MetadatiPDF{})
	aCapo := testoFinto("PART. SIMILE\n7120012\nPART. N° 7120001A1", "", worker.MetadatiPDF{})
	nelCampo := testoFinto("SCALA 1:1", "", worker.MetadatiPDF{})
	nelCampo.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "PART. N°", Valore: "7120001A1   SIMILAR PART 7120012",
		Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{620, 530, 655, 543}}}
	for _, x := range []struct {
		nome string
		tp   worker.TestoPDF
	}{{"accanto", accanto}, {"a capo", aCapo}, {"nel campo", nelCampo}} {
		fatti := fattiPDF(t, x.tp)
		l := classificazione.LettureDelPDF(m, fatti, "tavola 1.pdf")
		var codici []string
		for _, y := range l.Letture {
			codici = append(codici, y.Codice)
		}
		// 7120001 di famiglia, e 7120001A1 com'e' scritto (generico: una chiave); 7120012 mai
		if strings.Join(codici, " ") != "7120001 7120001A1" || l.Letture[0].Origine != OrigineFamiglia ||
			strings.Join(l.Simili, " ") != "7120012" || strings.Contains(strings.Join(l.Chiavi(), " "), "7120012") {
			t.Errorf("%s: letture %v, simili %v, chiavi %v", x.nome, codici, l.Simili, l.Chiavi())
		}
		if e := classificazione.EvidenzeTestoPDF(m, x.tp); len(e.Simili) != 1 || e.Simili[0].Codice != "7120012" ||
			e.Simili[0].Etichetta != classificazione.EtichettaSimile {
			t.Errorf("%s: la lettura del particolare simile: %+v", x.nome, e.Simili)
		}
		v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: "tavola 1.pdf", Direzione: "entrata",
			Motore: m, Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
		if v.Codice.Valore != "7120001" {
			t.Errorf("%s: il codice della valutazione: %+v", x.nome, v.Codice)
		}
		for _, e := range v.Codice.Evidenze {
			if e.Valore == "7120012" || e.Indizio == "7120012" {
				t.Errorf("%s: il particolare simile e' una lettura della valutazione: %+v", x.nome, e)
			}
		}

		// nel flusso: il prodotto con lo STEP autorizzato, in cui 7120012 e' un figlio diretto
		sc := nuovaScena(t)
		sc.s.Motore = m
		p := casoProdotto(sc, "7120001.stp")
		sc.autorizza(p, "7120001.stp", "#1")
		sc.disegnoLetto("tavola 1.pdf", fatti)
		d := sc.dest(sc.calcola(), "tavola 1.pdf")
		if strings.Join(d.Note, " | ") != "simile a 7120012" || strings.Contains(candidati(d), "7120012") {
			t.Errorf("%s: la destinazione: note %v, %s", x.nome, d.Note, candidati(d))
		}
		for _, k := range d.Codici {
			if k.Codice == "7120012" {
				t.Errorf("%s: il particolare simile e' un codice del file: %+v", x.nome, d.Codici)
			}
		}
		if len(d.Candidati) == 0 || d.Candidati[0].Codice != "7120001" {
			t.Errorf("%s: il disegno va al prodotto del suo cartiglio: %s", x.nome, candidati(d))
		}
	}

	// il segnaposto del modello non da' niente, e non si prende il codice della riga dopo
	for _, testo := range []string{
		"PARTICOLARE SIMILE / SIMILAR PART\nInserire codice particolare simile\n7120001A1",
		"PARTICOLARE SIMILE / SIMILAR PART Inserire codice particolare simile\nPART. N° 7120001A1",
	} {
		l := classificazione.LettureDelPDF(m, fattiPDF(t, testoFinto(testo, "", worker.MetadatiPDF{})), "tavola 1.pdf")
		var codici []string
		for _, y := range l.Letture {
			codici = append(codici, y.Codice)
		}
		if len(l.Simili) != 0 || strings.Join(codici, " ") != "7120001 7120001A1" {
			t.Errorf("il segnaposto %q: simili %v, letture %v", testo, l.Simili, codici)
		}
	}

	// il cartiglio a tabella con il campo simile vuoto: il codice sotto le etichette e' quello del file
	tabella := fattiPDF(t, testoFinto("PART. N°   PARTICOLARE SIMILE / SIMILAR PART\n7120001A1", "", worker.MetadatiPDF{}))
	l := classificazione.LettureDelPDF(m, tabella, "7120001.pdf")
	var codici []string
	for _, y := range l.Letture {
		codici = append(codici, y.Codice)
		if y.Fonte != classificazione.FonteTestoCartiglio {
			t.Errorf("il cartiglio a tabella: %s letto da %s", y.Codice, y.Fonte)
		}
	}
	if len(l.Simili) != 0 || strings.Join(codici, " ") != "7120001 7120001A1" {
		t.Errorf("il cartiglio a tabella: simili %v, letture %v", l.Simili, codici)
	}
	sc := nuovaScena(t)
	sc.s.Motore = m
	sc.prodotto("7120001")
	sc.disegnoLetto("7120001.pdf", tabella)
	c := sc.calcola()
	if a := c.Ancore["7120001"]; a.Livello != LivelloPiatta || len(a.Portatori) != 1 || strings.Join(a.Portatori[0].Regole, "+") != "A-P1+A-N1" {
		t.Errorf("il cartiglio a tabella ancora il prodotto: %s %+v", a.Livello, a.Portatori)
	}
	if d := sc.dest(c, "7120001.pdf"); len(d.Note) != 0 {
		t.Errorf("il cartiglio a tabella: note %v", d.Note)
	}
}

// Giro 4, fase 4.2, ritocco (dal verificatore finale della 4.2): il segnaposto del modello «Inserire codice
// particolare simile» non da' niente, ne' un simile ne' un codice, in tutte le forme in cui lo scrivono i disegni: da
// solo su una riga, dopo l'etichetta, prima del codice sulla stessa riga, in maiuscolo e in minuscolo, con la colonna
// del particolare simile prima o dopo quella del codice, spezzato su due righe, senza l'etichetta davanti, dentro il
// valore di un campo del cartiglio. Il caso dei disegni veri e' un cartiglio a tabella con la colonna del simile
// prima: «PARTICOLARE SIMILE / SIMILAR PART   PART. N°», a capo «Inserire codice particolare simile   7120001A1».
// Le parole «particolare simile» dentro il segnaposto non sono un'etichetta: prima la parola dopo (il codice del
// file, nella colonna accanto) diventava il valore del simile, e le letture restavano vuote con i simili [7120001
// 7120001A1]; e il segnaposto subito dopo l'etichetta si toglieva con tutta la sua riga, codice del file compreso.
// Adesso 7120001A1 resta il codice del disegno (7120001, e 7120001A1 com'e' scritto; con la revisione nel codice,
// come nelle regole dei clienti veri, 7120001 rev A1), senza note, e il disegno va al prodotto 7120001; il cartiglio
// a tabella con la colonna del simile prima e il segnaposto ancora il prodotto (A-P1). Un simile vero su un'altra
// riga resta una nota. La controprova (a mano): con la separaSimili di prima (il ramo HasPrefix «INSERIRE» e
// l'etichetta cercata anche dentro il segnaposto), e con quella nuova senza fineSegnaposto, falliscono le forme 1,
// 3, 7 e 9 in tutte le scritture, la spezzata, la prima dentro il campo e l'ancora del cartiglio a tabella.
func TestIlSegnapostoDelSimileNonDaNiente(t *testing.T) {
	m := motoreACME712(t)
	mRev := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})(?P<rev>[A-Z]\d)?`, RevNelCodice: true, Descrizione: "ACME 712 con la revisione", Esempio: "7120001A1"}}})
	segnaposti := []string{"Inserire codice particolare simile", "INSERIRE CODICE PARTICOLARE SIMILE",
		"inserire codice particolare simile", "Inserire il codice del part. simile"}
	forme := []string{
		// 1: il cartiglio a tabella dei disegni veri, la colonna del simile prima
		"PARTICOLARE SIMILE / SIMILAR PART   PART. N°\n{S}   7120001A1",
		// 2: la colonna del simile dopo quella del codice
		"PART. N°   PARTICOLARE SIMILE / SIMILAR PART\n7120001A1   {S}",
		// 3: l'etichetta da sola, sotto il segnaposto e il codice sulla stessa riga
		"PARTICOLARE SIMILE / SIMILAR PART\n{S}   7120001A1",
		// 4: il segnaposto da solo sulla sua riga, il codice sotto
		"PARTICOLARE SIMILE / SIMILAR PART\n{S}\n7120001A1",
		// 5: il segnaposto dopo l'etichetta, il codice sotto da solo
		"PARTICOLARE SIMILE / SIMILAR PART {S}\n7120001A1",
		// 6: il segnaposto dopo l'etichetta, il campo del codice sotto
		"PARTICOLARE SIMILE / SIMILAR PART: {S}\nPART. N° 7120001A1",
		// 7: tutto su una riga, il simile prima
		"PART. SIMILE {S}   7120001A1",
		// 8: tutto su una riga, il simile dopo
		"PART. N° 7120001A1   PARTICOLARE SIMILE {S}",
		// 9: il segnaposto senza l'etichetta davanti, prima del codice
		"{S}   7120001A1",
		// 10: il segnaposto in fondo
		"PART. N° 7120001A1\nSCALA 1:1\n{S}",
	}
	type caso struct {
		nome string
		tp   worker.TestoPDF
	}
	var casi []caso
	for i, forma := range forme {
		for _, s := range segnaposti {
			testo := strings.ReplaceAll(forma, "{S}", s)
			casi = append(casi, caso{"forma " + strconv.Itoa(i+1) + " «" + testo + "»", testoFinto(testo, "", worker.MetadatiPDF{})})
		}
	}
	// il segnaposto spezzato su due righe dentro la cella della tabella
	casi = append(casi, caso{"spezzato", testoFinto("PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire codice\nparticolare simile   7120001A1", "", worker.MetadatiPDF{})})
	// dentro il valore di un campo del cartiglio, prima o dopo il codice
	for _, valore := range []string{"Inserire codice particolare simile   7120001A1", "7120001A1   INSERIRE CODICE PARTICOLARE SIMILE"} {
		tp := testoFinto("SCALA 1:1", "", worker.MetadatiPDF{})
		tp.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "PART. N°", Valore: valore,
			Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{620, 530, 655, 543}}}
		casi = append(casi, caso{"nel campo «" + valore + "»", tp})
	}

	for _, x := range casi {
		fatti := fattiPDF(t, x.tp)
		l := classificazione.LettureDelPDF(m, fatti, "tavola 1.pdf")
		var codici []string
		for _, y := range l.Letture {
			codici = append(codici, y.Codice)
			if y.Fonte != classificazione.FonteTestoCartiglio {
				t.Errorf("%s: %s letto da %s", x.nome, y.Codice, y.Fonte)
			}
		}
		if len(l.Simili) != 0 || strings.Join(codici, " ") != "7120001 7120001A1" || l.Letture[0].Origine != OrigineFamiglia {
			t.Errorf("%s: simili %v, letture %v", x.nome, l.Simili, codici)
			continue
		}
		if e := classificazione.EvidenzeTestoPDF(m, x.tp); len(e.Simili) != 0 {
			t.Errorf("%s: le letture del particolare simile: %+v", x.nome, e.Simili)
		}
		v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: "tavola 1.pdf", Direzione: "entrata",
			Motore: m, Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
		if v.Codice.Valore != "7120001" {
			t.Errorf("%s: il codice della valutazione: %+v", x.nome, v.Codice)
		}
		// con la revisione nel codice, come nelle regole dei clienti veri: 7120001A1 e' 7120001 rev A1
		if lr := classificazione.LettureDelPDF(mRev, fatti, "tavola 1.pdf"); len(lr.Simili) != 0 || len(lr.Letture) == 0 ||
			lr.Letture[0].Codice != "7120001" || lr.Letture[0].Rev != "A1" {
			t.Errorf("%s: con la revisione nel codice: simili %v, letture %+v", x.nome, lr.Simili, lr.Letture)
		}

		// nel flusso: il disegno va al prodotto del suo cartiglio, senza note
		sc := nuovaScena(t)
		sc.s.Motore = m
		p := casoProdotto(sc, "7120001.stp")
		sc.autorizza(p, "7120001.stp", "#1")
		sc.disegnoLetto("tavola 1.pdf", fatti)
		d := sc.dest(sc.calcola(), "tavola 1.pdf")
		if len(d.Note) != 0 || len(d.Candidati) == 0 || d.Candidati[0].Codice != "7120001" {
			t.Errorf("%s: la destinazione: note %v, %s", x.nome, d.Note, candidati(d))
		}
	}

	// il cartiglio a tabella dei disegni veri, con il nome del prodotto: il suo disegno ne e' l'ancora piatta (A-P1)
	tabella := fattiPDF(t, testoFinto("PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire codice particolare simile   7120001A1", "", worker.MetadatiPDF{}))
	sc := nuovaScena(t)
	sc.s.Motore = m
	sc.prodotto("7120001")
	sc.disegnoLetto("7120001.pdf", tabella)
	c := sc.calcola()
	if a := c.Ancore["7120001"]; a.Livello != LivelloPiatta || len(a.Portatori) != 1 || strings.Join(a.Portatori[0].Regole, "+") != "A-P1+A-N1" {
		t.Errorf("il cartiglio a tabella con il segnaposto ancora il prodotto: %s %+v", a.Livello, a.Portatori)
	}
	if d := sc.dest(c, "7120001.pdf"); len(d.Note) != 0 {
		t.Errorf("il cartiglio a tabella con il segnaposto: note %v", d.Note)
	}

	// e un simile vero accanto al segnaposto di un'altra riga resta una nota: il segnaposto non spegne l'etichetta
	accanto := fattiPDF(t, testoFinto("PART. SIMILE 7120012\nPART. N° 7120001A1\nInserire codice particolare simile", "", worker.MetadatiPDF{}))
	if l := classificazione.LettureDelPDF(m, accanto, "tavola 1.pdf"); strings.Join(l.Simili, " ") != "7120012" {
		t.Errorf("il simile vero accanto al segnaposto: simili %v", l.Simili)
	}
}

// Giro 4, fase 4.2, secondo ritocco (dal verificatore del primo ritocco; risposta 4 del 29/09, domanda 3 del giro 4 A
// finche' non risponde): il segnaposto e' soltanto la frase del modello, «Inserire»/«Insert» seguito dalle parole
// della lista chiusa del segnaposto vero (qui, il, di, del, codice, numero, the, code, of, ...) e dall'etichetta del
// simile. Il segnaposto vuoto di un ALTRO campo messo prima dell'etichetta vera («Inserire trattamento», «Inserire
// materiale», «Insert treatment», la sola parola INSERT, una nota «INSERIRE BOCCOLE A PRESSIONE»), sulla stessa riga
// o sulla riga prima, non spegne l'etichetta: 7120012 e' il simile e le letture restano 7120001 e 7120001A1, come
// nella 4.2; nel flusso la destinazione ha la nota «simile a 7120012» e nessun candidato verso 7120012, che pure e' un
// figlio diretto dello STEP autorizzato. Lo stesso con il segnaposto del simile sulla riga prima, anche bilingue: non
// si prende l'etichetta vera sotto. Il segnaposto bilingue («Inserire codice particolare simile / similar part»,
// anche in maiuscolo) consuma anche l'etichetta gemella dopo la barra, e la forma lunga «Inserire qui il numero di
// codice del particolare simile» (anche spezzata, anche in inglese) e' un segnaposto: 7120001A1 resta il codice del
// disegno (con la revisione nel codice, 7120001 rev A1), senza simili ne' note, e il disegno va al prodotto 7120001.
// Lo stesso dentro il valore di un campo del cartiglio. Le controprove (a mano, con la segnapostoSimile cambiata):
// con quella del primo ritocco (fino a 4 parole qualsiasi senza cifre, anche a capo, e senza la gemella) falliscono
// 19 casi su 22, tutti tranne i due segnaposti del simile sulla riga prima e la sola parola INSERT con l'etichetta
// bilingue; con parole qualsiasi senza cifre al posto della lista chiusa, i segnaposti degli altri campi e i due
// segnaposti del simile sulla riga prima (le parole qualsiasi arrivano fino all'etichetta vera); senza almeno una
// parola, le tre della sola parola INSERT; senza la gemella, le cinque bilingui; con la gemella solo sulla stessa
// riga, la bilingue spezzata dopo la barra.
func TestIlSegnapostoDiUnAltroCampoNonSpegneIlSimile(t *testing.T) {
	m := motoreACME712(t)
	mRev := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})(?P<rev>[A-Z]\d)?`, RevNelCodice: true, Descrizione: "ACME 712 con la revisione", Esempio: "7120001A1"}}})
	type caso struct {
		nome   string
		tp     worker.TestoPDF
		simile string // il simile atteso, "" per nessuno
	}
	var casi []caso
	for _, x := range []struct{ nome, testo, simile string }{
		// il segnaposto di un altro campo prima dell'etichetta vera: il simile resta
		{"trattamento, la riga prima", "TRATTAMENTO Inserire trattamento\nPARTICOLARE SIMILE 7120012\nPART. N° 7120001A1", "7120012"},
		{"trattamento, la stessa riga", "Inserire trattamento   PARTICOLARE SIMILE 7120012   PART. N° 7120001A1", "7120012"},
		{"materiale, il valore sotto", "MATERIALE / MATERIAL\nInserire materiale\nPARTICOLARE SIMILE / SIMILAR PART\n7120012\nPART. N°\n7120001A1", "7120012"},
		{"treatment, la riga prima", "TREATMENT Insert treatment\nSIMILAR PART 7120012\nPART No. 7120001A1", "7120012"},
		{"treatment, la stessa riga", "TREATMENT Insert treatment   SIMILAR PART 7120012   PART No. 7120001A1", "7120012"},
		{"la sola parola INSERT, la riga prima", "INSERT\nPARTICOLARE SIMILE 7120012\nPART. N° 7120001A1", "7120012"},
		{"la sola parola INSERT, l'etichetta bilingue", "INSERT\nPARTICOLARE SIMILE / SIMILAR PART 7120012\nPART. N° 7120001A1", "7120012"},
		{"la sola parola INSERT, la stessa riga", "DESCRIZIONE INSERT   PARTICOLARE SIMILE 7120012   PART. N° 7120001A1", "7120012"},
		{"una nota con INSERIRE", "NOTA: INSERIRE BOCCOLE A PRESSIONE\nPARTICOLARE SIMILE 7120012\nPART. N° 7120001A1", "7120012"},
		{"il segnaposto del simile, la riga prima", "Inserire codice particolare simile\nPARTICOLARE SIMILE / SIMILAR PART 7120012\nPART. N° 7120001A1", "7120012"},
		{"il segnaposto bilingue, la riga prima", "Inserire codice particolare simile / similar part\nPARTICOLARE SIMILE / SIMILAR PART 7120012\nPART. N° 7120001A1", "7120012"},
		// il segnaposto bilingue: l'etichetta gemella dopo la barra e' del segnaposto
		{"bilingue", "PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire codice particolare simile / similar part   7120001A1", ""},
		{"bilingue in maiuscolo", "PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nINSERIRE CODICE PARTICOLARE SIMILE / SIMILAR PART   7120001A1", ""},
		{"bilingue sulla riga dell'etichetta", "PART. SIMILE Inserire codice particolare simile/similar part   7120001A1", ""},
		{"bilingue spezzato dopo la barra", "PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire codice particolare simile /\nsimilar part   7120001A1", ""},
		// la forma lunga
		{"lunga", "PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire qui il numero di codice del particolare simile   7120001A1", ""},
		{"lunga in maiuscolo", "PART. SIMILE   PART. N°\nINSERIRE QUI IL NUMERO DI CODICE DEL PARTICOLARE SIMILE   7120001A1", ""},
		{"lunga spezzata", "PARTICOLARE SIMILE / SIMILAR PART   PART. N°\nInserire qui il numero\ndi codice del particolare simile   7120001A1", ""},
		{"lunga in inglese", "SIMILAR PART   PART No.\nInsert here the code of the similar part   7120001A1", ""},
	} {
		casi = append(casi, caso{x.nome, testoFinto(x.testo, "", worker.MetadatiPDF{}), x.simile})
	}
	// dentro il valore di un campo del cartiglio
	for _, x := range []struct{ valore, simile string }{
		{"Insert treatment   SIMILAR PART 7120012   7120001A1", "7120012"},
		{"Inserire codice particolare simile / similar part   7120001A1", ""},
		{"Inserire qui il numero di codice del particolare simile   7120001A1", ""},
	} {
		tp := testoFinto("SCALA 1:1", "", worker.MetadatiPDF{})
		tp.Cartiglio = []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "PART. N°", Valore: x.valore,
			Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{620, 530, 655, 543}}}
		casi = append(casi, caso{"nel campo «" + x.valore + "»", tp, x.simile})
	}

	for _, x := range casi {
		fatti := fattiPDF(t, x.tp)
		l := classificazione.LettureDelPDF(m, fatti, "tavola 1.pdf")
		var codici []string
		for _, y := range l.Letture {
			codici = append(codici, y.Codice)
			if y.Fonte != classificazione.FonteTestoCartiglio {
				t.Errorf("%s: %s letto da %s", x.nome, y.Codice, y.Fonte)
			}
		}
		if strings.Join(codici, " ") != "7120001 7120001A1" || l.Letture[0].Origine != OrigineFamiglia ||
			strings.Join(l.Simili, " ") != x.simile || strings.Contains(strings.Join(l.Chiavi(), " "), "7120012") {
			t.Errorf("%s: letture %v, simili %v (atteso «%s»), chiavi %v", x.nome, codici, l.Simili, x.simile, l.Chiavi())
			continue
		}
		e := classificazione.EvidenzeTestoPDF(m, x.tp)
		if x.simile == "" && len(e.Simili) != 0 ||
			x.simile != "" && (len(e.Simili) != 1 || e.Simili[0].Codice != x.simile || e.Simili[0].Etichetta != classificazione.EtichettaSimile) {
			t.Errorf("%s: le letture del particolare simile: %+v", x.nome, e.Simili)
		}
		v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: "tavola 1.pdf", Direzione: "entrata",
			Motore: m, Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
		if v.Codice.Valore != "7120001" {
			t.Errorf("%s: il codice della valutazione: %+v", x.nome, v.Codice)
		}
		for _, ev := range v.Codice.Evidenze {
			if ev.Valore == "7120012" || ev.Indizio == "7120012" {
				t.Errorf("%s: il particolare simile e' una lettura della valutazione: %+v", x.nome, ev)
			}
		}
		// con la revisione nel codice: 7120001A1 e' 7120001 rev A1, e il simile resta lo stesso
		if lr := classificazione.LettureDelPDF(mRev, fatti, "tavola 1.pdf"); strings.Join(lr.Simili, " ") != x.simile || len(lr.Letture) == 0 ||
			lr.Letture[0].Codice != "7120001" || lr.Letture[0].Rev != "A1" {
			t.Errorf("%s: con la revisione nel codice: simili %v, letture %+v", x.nome, lr.Simili, lr.Letture)
		}

		// nel flusso: il prodotto con lo STEP autorizzato, in cui 7120012 e' un figlio diretto
		sc := nuovaScena(t)
		sc.s.Motore = m
		p := casoProdotto(sc, "7120001.stp")
		sc.autorizza(p, "7120001.stp", "#1")
		sc.disegnoLetto("tavola 1.pdf", fatti)
		d := sc.dest(sc.calcola(), "tavola 1.pdf")
		nota := ""
		if x.simile != "" {
			nota = classificazione.NotaSimile(x.simile)
		}
		if strings.Join(d.Note, " | ") != nota || strings.Contains(candidati(d), "7120012") || len(d.Candidati) == 0 ||
			d.Candidati[0].Codice != "7120001" {
			t.Errorf("%s: la destinazione: note %v, %s", x.nome, d.Note, candidati(d))
		}
		for _, k := range d.Codici {
			if k.Codice == "7120012" {
				t.Errorf("%s: il particolare simile e' un codice del file: %+v", x.nome, d.Codici)
			}
		}
	}
}

// Giro 4, fase 4.2, ritocco (P33, FP5; piano del giro 4, 4.2: «testo altrove = chiave»): un codice che viene
// soltanto dal testo del corpo del PDF, fuori dal cartiglio, e' una chiave di ricerca nell'indice, non il codice del
// file. Il candidato si mostra, ma non si preseleziona nemmeno verso 7120010, figlio diretto accettato dello STEP
// autorizzato del prodotto, che il sostegno dello STEP autorizzato ce l'ha: il legame fra il file e il pezzo sarebbe
// un numero citato in una nota. Tre scene con «vista generale.pdf», che nel nome non ha codici: l'evidenza del corpo
// data a mano, le letture vere di una nota nel corpo della pagina (LettureDelPDF), il corpo insieme al titolo dei
// metadati (due fonti che non sostengono non ne fanno una che sostiene). Le controprove: lo stesso bersaglio con il
// codice nel nome del file (anche con il corpo che lo ripete) o nel cartiglio si preseleziona; e, a mano, con la
// soloIndizi di prima (solo i metadati e l'OCR) le tre scene escono preselezionabili.
func TestIlTestoDelCorpoNonPreseleziona(t *testing.T) {
	corpo := EvidenzaContenutoPDF{Fonte: FontePDFTesto, Codice: "7120010", Pagina: 2, Origine: OrigineFamiglia}
	scene := []struct {
		nome, fonti string
		prepara     func(sc *scena)
	}{
		{"l'evidenza del corpo", FonteTestoPDF, func(sc *scena) {
			sc.disegno("vista generale.pdf")
			sc.f("vista generale.pdf").EvidenzePDF = []EvidenzaContenutoPDF{corpo}
		}},
		{"le letture del corpo", FonteTestoPDF, func(sc *scena) {
			sc.s.Motore = motoreACME712(t)
			sc.disegnoLetto("vista generale.pdf", fattiPDF(t, testoFinto("SCALA 1:1", "NOTA: montare con 7120010", worker.MetadatiPDF{})))
		}},
		{"il corpo e il titolo", FonteMetadatiPDF + " " + FonteTestoPDF, func(sc *scena) {
			sc.disegno("vista generale.pdf")
			sc.f("vista generale.pdf").EvidenzePDF = []EvidenzaContenutoPDF{corpo, {Fonte: FontePDFMetadati, Codice: "7120010"}}
		}},
	}
	for _, x := range scene {
		sc, _ := scenaPreselezionabile(t)
		x.prepara(sc)
		d := sc.dest(sc.calcola(), "vista generale.pdf")
		if len(d.Candidati) == 0 {
			t.Errorf("%s: il corpo deve dare un candidato mostrato: %s %v", x.nome, d.Esito, d.Evidenze)
			continue
		}
		top := d.Candidati[0]
		if candidati(d) != "1 7120010 dest_nodo_diretto_nome" || d.Preselezionabile || d.Stato != "da_scegliere" || top.DalContenuto ||
			contiene(top.Sostegno, SostegnoContenuto) || strings.Join(top.Fonti, " ") != x.fonti || len(top.Discordanze) != 0 {
			t.Errorf("%s: %s, preselezionabile %v, stato %s, fonti %v, sostegno %v, discordanze %v", x.nome, candidati(d),
				d.Preselezionabile, d.Stato, top.Fonti, top.Sostegno, top.Discordanze)
		}
		// il bersaglio ha il sostegno che basterebbe: a fermare la preselezione e' la fonte, non il bersaglio
		if !contiene(top.Sostegno, SostegnoStepAutorizzato) || !sostegnoBasta(top.Sostegno) || !strings.HasPrefix(top.Chiave, "componente:") {
			t.Errorf("%s: il bersaglio della scena: chiave %s, sostegno %v", x.nome, top.Chiave, top.Sostegno)
		}
		if len(d.Codici) != 1 || d.Codici[0].Codice != "7120010" || d.Codici[0].DalContenuto || len(d.Discordanze) != 0 {
			t.Errorf("%s: i codici del file: %+v, discordanze %v", x.nome, d.Codici, d.Discordanze)
		}
		if e := strings.Join(top.Evidenze, " | "); !strings.Contains(e, "il testo del PDF cita 7120010 (pagina ") {
			t.Errorf("%s: l'evidenza del corpo: %s", x.nome, e)
		}
	}

	// le controprove: il codice nel nome, da solo e con il corpo che lo ripete, e il cartiglio al posto del corpo
	sc, _ := scenaPreselezionabile(t)
	if d := sc.dest(sc.calcola(), "7120010.pdf"); candidati(d) != "1 7120010 dest_nodo_diretto_nome" || !d.Preselezionabile {
		t.Errorf("il codice nel nome: %s, preselezionabile %v", candidati(d), d.Preselezionabile)
	}
	sc, _ = scenaPreselezionabile(t)
	sc.f("7120010.pdf").EvidenzePDF = []EvidenzaContenutoPDF{corpo}
	if d := sc.dest(sc.calcola(), "7120010.pdf"); candidati(d) != "1 7120010 dest_nodo_diretto_nome" || !d.Preselezionabile ||
		strings.Join(d.Candidati[0].Fonti, " ") != FonteNome+" "+FonteTestoPDF {
		t.Errorf("il codice nel nome e nel corpo: %s, preselezionabile %v, fonti %v", candidati(d), d.Preselezionabile, d.Candidati[0].Fonti)
	}
	sc, _ = scenaPreselezionabile(t)
	sc.disegno("vista generale.pdf")
	sc.f("vista generale.pdf").EvidenzePDF = []EvidenzaContenutoPDF{{Fonte: FontePDFCartiglio, Codice: "7120010", Pagina: 1, Origine: OrigineFamiglia}}
	if d := sc.dest(sc.calcola(), "vista generale.pdf"); candidati(d) != "1 7120010 dest_nodo_diretto_contenuto" || !d.Preselezionabile {
		t.Errorf("il cartiglio al posto del corpo: %s, preselezionabile %v", candidati(d), d.Preselezionabile)
	}
}

// Riscritta per lo Smistamento (giro 4, fase 4.2): prima fissava il punto d'aggancio vuoto (EvidenzeContenutoPDF
// non dava niente, e ogni PDF si comportava come un PDF senza testo con la frase «il PDF non ha evidenze dal
// contenuto»); adesso le evidenze vengono dalle letture normalizzate, e la frase e' quella dello stato del testo
// (le prove 163 e 164 guardano le ancore; questa la destinazione di un disegno dentro una struttura). Un PDF senza
// testo, con il testo non letto o illeggibile ha il codice solo dal nome, non «dal contenuto», e lo dice con la
// frase del suo stato; uno letto senza codici dice «il testo del PDF non porta codici del cliente». Quando il
// cartiglio porta un codice di famiglia, e' un'evidenza indipendente: il candidato coerente con il cartiglio viene
// primo, la discordanza con il nome non si preseleziona, e nessuna frase dice che il PDF non ha evidenze.
func TestSenzaEvidenzeIlPdfSiComportaComeUnPdfSenzaTesto(t *testing.T) {
	muto := testoFinto("", "", worker.MetadatiPDF{})
	muto.OCR = worker.OCRPDF{Stato: worker.OCRSpento}
	costruisci := func(fatti json.RawMessage) *scena {
		sc, _ := scenaPreselezionabile(t)
		sc.s.Motore = motoreACME712(t)
		sc.accetta("7120001.stp", "#3", sc.componente("7120012", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
		sc.disegnoLetto("7120012.pdf", fatti)
		return sc
	}
	for _, x := range []struct {
		nome  string
		fatti json.RawMessage
		frase string
	}{
		{"senza testo", fattiPDF(t, muto), "serve l'OCR, spento sul worker: il suo codice viene solo dal nome"},
		{"non letto", json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`), "da rianalizzare: il suo codice viene solo dal nome"},
		{"illeggibile", json.RawMessage(`{"errore_pdf": "broken"}`), "il PDF non si apre: il suo codice viene solo dal nome"},
		{"letto senza codici", fattiPDF(t, testoFinto("SCALA 1:2", "", worker.MetadatiPDF{})), "il testo del PDF non porta codici del cliente: il suo codice viene solo dal nome"},
	} {
		sc := costruisci(x.fatti)
		if n := len(sc.f("7120012.pdf").EvidenzePDF); n != 0 {
			t.Fatalf("%s: la scena: %d evidenze", x.nome, n)
		}
		d := sc.dest(sc.calcola(), "7120012.pdf")
		if candidati(d) != "1 7120012 dest_nodo_diretto_nome" || d.Candidati[0].DalContenuto || strings.Join(d.Candidati[0].Fonti, " ") != FonteNome {
			t.Errorf("%s: il PDF senza evidenze: %s %+v", x.nome, candidati(d), d.Candidati)
		}
		if !strings.Contains(evidenzeIn(d), x.frase) || strings.Contains(evidenzeIn(d), "non ha evidenze dal contenuto") {
			t.Errorf("%s: la destinazione dice lo stato del testo: %s", x.nome, evidenzeIn(d))
		}
	}
	// con un codice di famiglia nel cartiglio, diverso dal nome
	sc := costruisci(fattiPDF(t, testoFinto("DISEGNO N. 7120010", "", worker.MetadatiPDF{})))
	d := sc.dest(sc.calcola(), "7120012.pdf")
	if len(d.Candidati) != 2 || d.Candidati[0].Codice != "7120010" || d.Candidati[0].Regola != "dest_nodo_diretto_contenuto" ||
		d.Candidati[1].Codice != "7120012" || d.Preselezionabile || !contiene(d.Discordanze, DiscFontiDiverse) {
		t.Errorf("il cartiglio contro il nome: %s, preselezionabile %v, %v", candidati(d), d.Preselezionabile, d.Discordanze)
	}
	if e := evidenzeIn(d); strings.Contains(e, "non porta codici") || strings.Contains(e, "non ha evidenze") ||
		!strings.Contains(e, "il cartiglio del PDF dice 7120010 (pagina 1)") {
		t.Errorf("accanto al cartiglio nessuna frase dice che il PDF non ha evidenze: %s", e)
	}
}
