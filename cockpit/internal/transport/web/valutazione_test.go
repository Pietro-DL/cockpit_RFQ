package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// Il punto 14 del censimento, «esito proposto X · score N» nella prova delle regole dell'anagrafica
// (anagrafica.html, `$t.Confidenza`), e' la confidenza dell'ESITO del triage di una mail, la stessa grandezza
// che la posta (M1) mostra a livelli nell'Inbox. M1 non l'ha toccato: resta «score N» con la dimensione.

// TestNessunTemplateMostraPercentuali (Smistamento, prova 228, A5.14.5, U-C1, U-C6): nessun template scrive
// uno score come percentuale, nessun «punteggio» o «confidenza» resta visibile (nel testo o in un `title`)
// senza la dimensione, e ogni `<span class="punteggio">` scritto a mano dice «score». Gli score dei file
// passano dalla funzione `score` e dal frammento «dimensione»; quelli dell'aggancio delle mail dalle carte
// a livelli di M1. All'unione delle due linee e' caduto l'elenco dei punti lasciati alla posta: l'unico
// rimasto, il candidato di richiesta del pannello, dice ora «richiesta · score N».
//
// Giro di correzione di F3: la prova guarda anche DENTRO le azioni. Prima le sostituiva con «§» e poi
// guardava il testo, e un numero nudo (`{{.Punteggio}} · {{.Evidenza}}`, `<strong>{{$t.Confidenza}}</strong>`)
// o `{{printf "%d%%" .Confidenza}}` passavano. Adesso un campo di score (.Punteggio, .Confidenza,
// .TriageConfidenza, .Score) stampato da solo o con printf deve avere «score» subito prima; ogni `score`,
// e ogni numero preceduto da «score», deve avere la dimensione nominata sulla stessa riga («codice ·»,
// «tipo ·», «rev ·», «richiesta ·», «esito proposto … ·»), tranne dentro il frammento «dimensione», che la
// riceve per nome;
// nessuna printf scrive «%%».
func TestNessunTemplateMostraPercentuali(t *testing.T) {
	commento := regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)
	azione := regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	percentuale := regexp.MustCompile(`§\s*%|[0-9]\s*%`)
	parola := regexp.MustCompile(`(?i)punteggi|confidenz`)
	spanScore := regexp.MustCompile(`<span class="punteggio"[^>]*>(.{0,6})`)
	campoScore := regexp.MustCompile(`\.(Punteggio|Confidenza|TriageConfidenza|Score)\b`)
	nudo := regexp.MustCompile(`^(\$\w*)?(\.\w+)+$`)
	controllo := regexp.MustCompile(`^(if|else|range|with|end|define|template|block|break|continue)\b|:=|^\$\w+\s*=`)
	conDimensione := regexp.MustCompile(`(?i)(codice|tipo|rev|richiesta|esito proposto[^·&]*)\s*(·|&middot;)\s*(score\s*)?$`)
	definizione := regexp.MustCompile(`\{\{-?\s*define\s+"(\w+)"`)
	voci, err := fs.Glob(risorse.FS, "web/templates/*.html")
	if err != nil || len(voci) < 10 {
		t.Fatalf("template: %v %v", voci, err)
	}
	for _, f := range voci {
		b, _ := fs.ReadFile(risorse.FS, f)
		// i commenti dei template non arrivano al browser; le azioni diventano «§», con le righe al loro posto
		s := commento.ReplaceAllStringFunc(string(b), func(m string) string { return strings.Repeat("\n", strings.Count(m, "\n")) })
		grezze := strings.Split(s, "\n")
		s = azione.ReplaceAllStringFunc(s, func(m string) string { return "§" + strings.Repeat("\n", strings.Count(m, "\n")) })
		def := "" // il frammento in cui si e': le define non si annidano
		for i, riga := range strings.Split(s, "\n") {
			for _, m := range definizione.FindAllStringSubmatch(grezze[i], -1) {
				def = m[1]
			}
			// dentro le azioni della riga
			for _, pos := range azione.FindAllStringIndex(grezze[i], -1) {
				c := strings.TrimSpace(strings.Trim(grezze[i][pos[0]+2:pos[1]-2], "-"))
				prima := azione.ReplaceAllString(grezze[i][:pos[0]], "§")
				switch {
				case strings.Contains(c, "printf") && strings.Contains(c, "%%"):
					t.Errorf("%s:%d: una percentuale scritta con printf: %s", f, i+1, strings.TrimSpace(grezze[i]))
				case controllo.MatchString(c):
				case strings.HasPrefix(c, "score "):
					if def != "dimensione" && !conDimensione.MatchString(prima) {
						t.Errorf("%s:%d: uno score senza la dimensione di cui parla («codice ·», «tipo ·», …) prima: %s", f, i+1, strings.TrimSpace(grezze[i]))
					}
				case campoScore.MatchString(c) && (nudo.MatchString(c) || strings.HasPrefix(c, "printf")):
					switch {
					case !strings.HasSuffix(strings.TrimSpace(prima), "score"):
						t.Errorf("%s:%d: un numero di score stampato senza «score» (si passa da `score`): %s", f, i+1, strings.TrimSpace(grezze[i]))
					case def != "dimensione" && !conDimensione.MatchString(prima):
						t.Errorf("%s:%d: «score N» senza la dimensione di cui parla prima: %s", f, i+1, strings.TrimSpace(grezze[i]))
					}
				}
			}
			if percentuale.MatchString(riga) {
				t.Errorf("%s:%d: uno score come percentuale: %s", f, i+1, strings.TrimSpace(grezze[i]))
			}
			if parola.MatchString(strings.ReplaceAll(riga, `class="punteggio"`, "")) {
				t.Errorf("%s:%d: «punteggio»/«confidenza» senza la dimensione (si dice «score», con che cosa misura): %s", f, i+1, strings.TrimSpace(grezze[i]))
			}
			for _, m := range spanScore.FindAllStringSubmatch(riga, -1) {
				if !strings.HasPrefix(m[1], "score") {
					t.Errorf("%s:%d: un numero nel riquadro dello score senza la parola «score»: %s", f, i+1, strings.TrimSpace(grezze[i]))
				}
			}
		}
	}
}

// rendiDimensione esegue il frammento «dimensione» su una dimensione.
func rendiDimensione(t *testing.T, d vistaDimensione) string {
	t.Helper()
	var buf bytes.Buffer
	if err := serverTest(t).pagine["fascicolo.html"].ExecuteTemplate(&buf, "dimensione", d); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestIlFrammentoDimensioneDiceCheCosaMisura (prova 237, la parte che si prova senza browser; U-C1…U-C5): il
// frammento mostra «score N» con la regola in parole, «2 fonti» per le fonti concordi, il chip «fonti
// discordi» con le evidenze aperte, «nessuna evidenza» senza numero, una decisione senza score, e la nota
// delle righe ricostruite. Nessuna percentuale in nessun caso.
func TestIlFrammentoDimensioneDiceCheCosaMisura(t *testing.T) {
	riga := func(tipo, codice, rev, fonte string, conf int, dett, nome string) vistaValutazione {
		return valutazioneDi(&db.DocumentoProposta{TipoProposto: db.TipoDocumento(tipo), Codice: txtT(codice), Rev: txtT(rev),
			Fonte: db.FonteProposta(fonte), Confidenza: int16(conf), Dettagli: json.RawMessage(dett)}, nome)
	}
	casi := []struct {
		nome     string
		d        vistaDimensione
		attesi   []string
		vietati  []string
		evidenze string // "", "chiuse", "aperte"
	}{
		{"unica, dal nome", dimVista("Codice letto", riga("cad_3d", "7120001A", "1", "nome_file", 90, `{}`, "7120001A_1.stp").Codice),
			[]string{`<span class="dim-nome">Codice letto</span>`, `<b class="mono">7120001A</b>`, `title="` + titoloScoreS1 + `">score 45</span>`,
				"nome del file", "(lettura precedente, ricostruita)"}, []string{"fonti discordi", "fonti</span>"}, ""},
		{"concorde, estensione e contenuto", dimVista("Tipo", riga("cad_3d", "7120001A", "", "step", 95, `{"product_step": "7120001A", "struttura": {}}`, "7120001A.stp").Tipo),
			[]string{`<b class="mono">3D</b>`, ">score 95</span>", "estensione .stp", "2 fonti", "contenuto dello STEP letto"}, []string{"fonti discordi"}, "chiuse"},
		{"discorde, la radice di famiglia contro il nome", dimVista("Codice letto", riga("cad_3d", "7120001", "", "regola_cliente", 80, `{"famiglia": "ACME 712", "testo": "7120001"}`, "7120001A_1.stp").Codice),
			[]string{`<b class="mono">7120001</b>`, ">score 80</span>", "radice dello STEP", "fonti discordi", ">score 45</span> · nome del file", "famiglia «ACME 712»"}, nil, "aperte"},
		{"nessuna evidenza", dimVista("Tipo", riga("da_determinare", "", "", "estensione", 40, `{}`, "Capitolato.pdf").Tipo),
			[]string{"nessuna evidenza (PDF non ancora letto)"}, []string{"score", "<b"}, ""},
		{"un PDF letto che non si apre", dimVista("Tipo", riga("da_determinare", "", "", "estensione", 20, `{"errore_pdf": "x"}`, "Capitolato.pdf").Tipo),
			[]string{"nessuna evidenza (PDF letto, non si apre)"}, []string{"score", "<b", "non ancora letto"}, ""},
		{"decisa da una persona", dimVista("Codice letto", riga("disegno_2d", "7120010", "B", "operatore", 100, `{}`, "x.pdf").Codice),
			[]string{`<b class="mono">7120010</b>`, "deciso da una persona"}, []string{"score", "100", "ricostruita"}, ""},
		{"una dipendenza non e' una seconda fonte", dimVista("Codice letto", riga("cad_3d", "7120001A", "", "step", 95, `{"product_step": "7120001A"}`, "7120001A.stp").Codice),
			[]string{">score 45</span> · nome del file", "dipende dal nome del file"}, []string{"2 fonti", "fonti discordi"}, "chiuse"},
	}
	for _, c := range casi {
		h := rendiDimensione(t, c.d)
		haTesto(t, c.nome, h, c.attesi...)
		senzaTesto(t, c.nome, h, append(c.vietati, "%", "punteggio della", "Confidenza")...)
		switch c.evidenze {
		case "":
			senzaTesto(t, c.nome, h, "<details")
		case "chiuse":
			haTesto(t, c.nome, h, `<details class="dim-evidenze">`)
		case "aperte":
			haTesto(t, c.nome, h, `<details class="dim-evidenze" open>`)
		}
	}

	// una valutazione salvata (la scrivera' la F4) si legge com'e', senza la nota delle righe ricostruite
	v := classificazione.Valutazione{V: 1, Tabella: "S1", Da: "analisi", Tipo: classificazione.Dimensione{Valore: "cad_3d", Score: 95, Regola: "ext_3d",
		Stato: classificazione.StatoUnica, Evidenze: []classificazione.Evidenza{{Regola: "ext_3d", Fonte: "estensione", Valore: "cad_3d", Score: 95, Testo: ".stp"}}}}
	dett, _ := json.Marshal(map[string]any{"valutazione": v})
	salvata := valutazioneDi(&db.DocumentoProposta{TipoProposto: db.TipoDocumentoCad3d, Fonte: db.FontePropostaNomeFile, Confidenza: 90, Dettagli: dett}, "7120001A_1.stp")
	h := rendiDimensione(t, dimVista("Tipo", salvata.Tipo))
	haTesto(t, "salvata", h, ">score 95</span> · estensione .stp")
	senzaTesto(t, "salvata", h, "ricostruita")
	if valutazioneDi(nil, "x.pdf").Tipo.ConValore() {
		t.Error("senza proposta non c'e' niente da mostrare")
	}
}

// TestLaLetturaDipendenteNonAlzaLoScoreNelFrammento (Domanda 7 = B, 27/09, il caso guida): la rev di
// «7120001A_1.stp» con il PRODUCT «7120001A_1» si mostra con la regola del nome, «score 40 · convenzione
// generica _1», e non «score 45 · rev dal PRODUCT dello STEP»; il PRODUCT resta fra le evidenze con lo score
// del nome, e la nota dice che dipende dal nome e non vale piu' del nome. Una fonte sola, nessun «2 fonti».
// La stessa riga salvata v1 (con la rev vinta dal PRODUCT) si mostra allo stesso modo.
func TestLaLetturaDipendenteNonAlzaLoScoreNelFrammento(t *testing.T) {
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: "7120001A_1.stp", Direzione: "entrata",
		Esito: &classificazione.Esito{Tipo: "cad_3d", Fonte: "step"}, Fatti: json.RawMessage(`{"product_step": "7120001A_1"}`)})
	salvata, _ := json.Marshal(map[string]any{"valutazione": v, "product_step": "7120001A_1"})
	// la stessa lettura come la scriveva la v1: la dimensione vinta dal PRODUCT, le evidenze con lo score della tabella
	v1 := v
	v1.V = 1
	v1.Rev = classificazione.Dimensione{Valore: "1", Score: 45, Regola: "rev_step_product", Stato: classificazione.StatoUnica,
		Evidenze: append([]classificazione.Evidenza(nil), v.Rev.Evidenze...)}
	for i := range v1.Rev.Evidenze {
		if e := &v1.Rev.Evidenze[i]; e.ScoreRegola > 0 {
			e.Score, e.ScoreRegola = e.ScoreRegola, 0
		}
	}
	vecchia, _ := json.Marshal(map[string]any{"valutazione": v1, "product_step": "7120001A_1"})
	for nome, dett := range map[string][]byte{"v2": salvata, "v1": vecchia} {
		vv := valutazioneDi(&db.DocumentoProposta{TipoProposto: db.TipoDocumentoCad3d, Codice: txtT("7120001A"), Rev: txtT("1"),
			Fonte: db.FontePropostaNomeFile, Confidenza: 45, Dettagli: dett}, "7120001A_1.stp")
		h := rendiDimensione(t, dimVista("Rev", vv.Rev))
		haTesto(t, nome, h, `<b class="mono">1</b> · <span class="punteggio" title="`+titoloScoreS1+`">score 40</span> · convenzione generica _1`,
			"rev dal PRODUCT dello STEP", "dipende dal nome del file: non è una seconda fonte e non vale più del nome, anche se la sua regola da sola varrebbe di più",
			`<details class="dim-evidenze">`)
		senzaTesto(t, nome, h, "score 45", "2 fonti", "fonti discordi", "ricostruita", "%")
		if vv.Rev.Fonti() != 1 {
			t.Errorf("%s: fonti %d, attesa una", nome, vv.Rev.Fonti())
		}
	}
	// una dipendenza da un'altra fonte si dice con la sua fonte
	e := classificazione.Evidenza{Regola: "pdf_termini_offerta_entrata", Valore: "commerciale", Score: 40, ScoreRegola: 50, DipendeDa: "estensione"}
	if got := dipendenzaEvidenza(e); got != "dipende da un'altra lettura («estensione»): non è una seconda fonte e non vale più di quella lettura, anche se la sua regola da sola varrebbe di più" {
		t.Errorf("dipendenza da un'altra fonte: %q", got)
	}
}

// TestLIndizioDellOcrSiMostraSenzaScore (Smistamento F9; decisioni del 27/09 «ter»: l'OCR e' un'evidenza, non
// un automatismo): un codice letto con l'OCR nel cartiglio di «7120010.pdf», diverso dal nome, si mostra fra le
// evidenze del codice con il codice, la fonte («letto con l'OCR») e «indizio, senza score», senza un numero e
// senza fare «fonti discordi». La dimensione resta quella del nome.
func TestLIndizioDellOcrSiMostraSenzaScore(t *testing.T) {
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	fatti := json.RawMessage(`{"codice_riconosciuto": "7120010", "testo_letto": 0, "testo_pdf": {"versione": 1, "estraibile": false,
		"frammenti": [{"pagina": 1, "zona": "basso_destra", "fonte": "ocr", "testo": "DISEGNO N. 7120011", "riquadro": [600, 500, 700, 512], "confidenza": 62}],
		"ocr": {"stato": "eseguito", "motore": "tesseract (pymupdf)"}}}`)
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: "7120010.pdf", Direzione: "entrata",
		Motore: m, Esito: &classificazione.Esito{Tipo: "da_determinare", Fonte: "nome_file"}, Fatti: fatti})
	dett, _ := json.Marshal(map[string]any{"valutazione": v})
	vv := valutazioneDi(&db.DocumentoProposta{TipoProposto: db.TipoDocumentoDaDeterminare, Codice: txtT("7120010"),
		Fonte: db.FontePropostaNomeFile, Confidenza: 70, Dettagli: dett}, "7120010.pdf")
	h := rendiDimensione(t, dimVista("Codice letto", vv.Codice))
	haTesto(t, "indizio OCR", h, `<b class="mono">7120010</b> · <span class="punteggio" title="`+titoloScoreS1+`">score 70</span>`,
		`<span class="mono">7120011</span> · <span class="k muto"`, "indizio, senza score", "codice letto con l&#39;OCR",
		"letto con l&#39;OCR", `famiglia «ACME 712»`)
	senzaTesto(t, "indizio OCR", h, "score 0", "fonti discordi", "2 fonti", "%")
}

// TestLaFunzioneScoreNonScriveMaiUnaPercentuale (U-C1): «score N» con il tooltip, per le dimensioni, le
// evidenze e i numeri dei codici; niente per una dimensione senza evidenza o per una decisione.
func TestLaFunzioneScoreNonScriveMaiUnaPercentuale(t *testing.T) {
	d := classificazione.Dimensione{Valore: "7120001", Score: 80, Regola: "step_radice_famiglia", Stato: classificazione.StatoUnica}
	casi := map[string]struct {
		x      any
		atteso string
	}{
		"dimensione":       {d, `<span class="punteggio" title="` + titoloScoreS1 + `">score 80</span>`},
		"vista":            {vistaDimensione{Quale: "codice", D: d}, `>score 80</span>`},
		"evidenza":         {classificazione.Evidenza{Regola: "ext_3d", Valore: "cad_3d", Score: 95}, `>score 95</span>`},
		"numero di codice": {int16(30), `<span class="punteggio" title="` + titoloScore + `">score 30</span>`},
		"intero":           {80, `>score 80</span>`},
		"nessuna":          {classificazione.Dimensione{Stato: classificazione.StatoNessuna}, ""},
		"decisa":           {vistaDimensione{Quale: "codice", D: d, Decisa: true}, ""},
		"evidenza vuota":   {classificazione.Evidenza{Regola: "ext_pdf"}, ""},
	}
	for nome, c := range casi {
		s := string(scoreHTML(c.x))
		if (c.atteso == "" && s != "") || !strings.Contains(s, c.atteso) || strings.Contains(s, "%") {
			t.Errorf("%s: %q, atteso %q", nome, s, c.atteso)
		}
	}
}
