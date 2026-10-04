// L1 — le tabelle della mail con la loro origine (giro 5, A1b.3; piano A, 5.4.4): le stesse tabelle e gli stessi
// testi di vista() e le stesse tabelle agganciate di Presenta (A1b-10), le righe, gli indici e il percorso di
// ogni cella (A1b-11, la forma di F-MAIL-4), le tabelle che non combaciano o stanno a cavallo del taglio
// (A1b-12, F-MAIL-5).
//
// Tutti i dati sono sintetici: ACME, Fornitore Esempio, codici di fantasia (ACME1111, PB07XX0001…), indirizzi
// @acme.example, l'intestazione inventata «Q.TA». Le forme sono quelle di Outlook (Word HTML, una cella per
// riga nel testo, CRLF), il contenuto no: il repository è pubblico.
package lettura

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// celleDiOrigine: i testi delle celle riga per riga, troncati come li tronca vista() per la pagina.
func celleDiOrigine(tb TabellaOrigine) [][]string {
	var out [][]string
	for _, c := range tb.Celle {
		for len(out) <= c.Riga {
			out = append(out, nil)
		}
		testo, _ := tronca(c.Testo, maxRuneCella)
		out[c.Riga] = append(out[c.Riga], testo)
	}
	return out
}

// paroleDelleRighe: le parole delle righe [da, a) del Taglio, sul corpo originale.
func paroleDelleRighe(corpo string, tg classificazione.Taglio, iv [2]int) []string {
	var out []string
	for k := iv[0]; k < iv[1]; k++ {
		r := tg.Righe[k]
		out = append(out, parole(corpo[r.Inizio:r.Fine])...)
	}
	return out
}

// conOrigine calcola le tabelle con l'origine come le calcolerà l'adattatore (il testo dall'HTML quando il
// corpo_testo è vuoto, il Taglio su quel testo) e controlla gli invarianti: le stesse tabelle di vista(),
// le stesse agganciate di Presenta, le parole delle righe uguali a quelle delle celle, le righe delle celle
// dentro quelle della tabella, il determinismo.
func conOrigine(t *testing.T, corpo, h string) (string, classificazione.Taglio, []TabellaOrigine) {
	t.Helper()
	testoDelTaglio := corpo
	if strings.TrimSpace(corpo) == "" {
		testoDelTaglio = TestoDaHTML(h)
	}
	tg := classificazione.TagliaCatenaConPosizioni(testoDelTaglio)
	got := TabelleConOrigine(testoDelTaglio, h, tg)
	if again := TabelleConOrigine(testoDelTaglio, h, tg); !reflect.DeepEqual(got, again) {
		t.Fatal("due letture dello stesso messaggio sono diverse")
	}

	// Le stesse tabelle di vista(), con gli stessi testi, span e numeri.
	var viste []*tabellaHTML
	if h != "" && len(h) <= LimiteHTML && contieneTabella(h) {
		viste = leggiHTML(h).tabelle
	}
	if len(got) != len(viste) {
		t.Fatalf("%d tabelle con l'origine, %d nella lettura dell'HTML", len(got), len(viste))
	}
	for i, tb := range got {
		v := viste[i].vista()
		if tb.Intestazione != v.Intestazione {
			t.Errorf("tabella %d: intestazione %v, la vista dice %v", i, tb.Intestazione, v.Intestazione)
		}
		k := 0
		for r, riga := range v.Righe {
			for cl, vc := range riga.Celle {
				if k >= len(tb.Celle) {
					t.Fatalf("tabella %d: mancano celle", i)
				}
				c := tb.Celle[k]
				testo, troncata := tronca(c.Testo, maxRuneCella)
				if c.Tabella != i || c.Riga != r || c.Cella != cl || testo != vc.Testo || troncata != vc.Troncata ||
					c.Colspan != vc.Colspan || c.Rowspan != vc.Rowspan || c.Numero != vc.Numero {
					t.Errorf("tabella %d, cella %d: %+v contro la vista %+v", i, k, c, vc)
				}
				k++
			}
		}
		if k != len(tb.Celle) {
			t.Errorf("tabella %d: %d celle in più della vista", i, len(tb.Celle)-k)
		}
	}

	// Le stesse tabelle agganciate di Presenta, nello stesso ordine (prima la parte fresca, poi la storia).
	c := verifica(t, corpo, h)
	var diPresenta []string
	for _, tb := range append(tabelle(c.Blocchi, OrigineHTML), tabelle(c.Storia, OrigineHTML)...) {
		diPresenta = append(diPresenta, fmt.Sprint(celle(tb)))
	}
	var agganciate []string
	for _, tb := range got {
		if tb.Agganciata {
			agganciate = append(agganciate, fmt.Sprint(celleDiOrigine(tb)))
		}
	}
	if !reflect.DeepEqual(agganciate, diPresenta) {
		t.Errorf("agganciate %q, Presenta ne mostra %q", agganciate, diPresenta)
	}

	// Nessuna parola persa: le parole delle righe di una tabella agganciata sono quelle delle sue celle; ogni
	// cella sta nelle righe della tabella; una cella che coincide ha le parole delle sue righe.
	for i, tb := range got {
		if !tb.Agganciata {
			if tb.Righe != nil {
				t.Errorf("tabella %d non agganciata con le righe %v", i, *tb.Righe)
			}
			for _, cl := range tb.Celle {
				if cl.Righe != nil || cl.Coincide {
					t.Errorf("tabella %d non agganciata, cella con le righe: %+v", i, cl)
				}
			}
			continue
		}
		if tb.ACavallo || tb.Righe == nil {
			t.Fatalf("tabella %d agganciata senza righe o a cavallo", i)
		}
		var dalleCelle []string
		for _, cl := range tb.Celle {
			w := parole(cl.Testo)
			dalleCelle = append(dalleCelle, w...)
			if len(w) == 0 {
				if cl.Righe != nil {
					t.Errorf("tabella %d: una cella vuota ha le righe %v", i, *cl.Righe)
				}
				continue
			}
			if cl.Righe == nil || cl.Righe[0] < tb.Righe[0] || cl.Righe[1] > tb.Righe[1] || cl.Righe[0] >= cl.Righe[1] {
				t.Errorf("tabella %d: righe della cella %+v fuori da %v", i, cl, *tb.Righe)
				continue
			}
			sueRighe := paroleDelleRighe(testoDelTaglio, tg, *cl.Righe)
			if cl.Coincide != reflect.DeepEqual(sueRighe, w) {
				t.Errorf("tabella %d: cella %q, Coincide %v, ma le sue righe dicono %q", i, cl.Testo, cl.Coincide, sueRighe)
			}
		}
		if righe := paroleDelleRighe(testoDelTaglio, tg, *tb.Righe); !reflect.DeepEqual(righe, dalleCelle) {
			t.Errorf("tabella %d: le righe dicono %q, le celle %q", i, righe, dalleCelle)
		}
	}
	return testoDelTaglio, tg, got
}

// rowspanEColspan: la tabella di TestColspanERowspanLimitati, con i codici di fantasia.
const rowspanEColspan = `<table>
<tr><td rowspan=99>ACME1111</td><td>Flangia</td><td>10</td></tr>
<tr><td>Dado</td><td>20</td></tr>
<tr><td colspan=2>Totale</td><td colspan=abc>30</td></tr>
</table>`

// TestLeTabelleConOrigineSonoQuelleDellaVista (A1b-10): sulle fixture di lettura (excel.html con il testo a TAB,
// con una cella per riga e senza testo) e su tabelle sintetiche, le tabelle con l'origine sono quelle della vista,
// con gli stessi testi, e quelle agganciate sono quelle che Presenta mostra.
func TestLeTabelleConOrigineSonoQuelleDellaVista(t *testing.T) {
	excel := leggiFixture(t, "excel.html")
	nuova := [][]string{{"ACME2020", "Vite", "5"}, {"ACME2121", "Dado", "6"}}
	vecchia := [][]string{{"ACME1010", "Flangia", "100"}, {"ACME1111", "Rondella", "200"}}
	dueTabelleHTML := "<p class=MsoNormal>Ecco la nuova lista:</p>" + tabellaHTMLDi(nuova) +
		`<div style="border:none;border-top:solid #E1E1E1 1.0pt"><p class=MsoNormal><b>Da:</b> Ufficio Acquisti</p></div>` +
		"<p class=MsoNormal>Vi chiediamo quotazione per:</p>" + tabellaHTMLDi(vecchia)
	dueTabelleTesto := "Ecco la nuova lista:\r\n" + testoTabDi(nuova) + "\r\n\r\n________________________________\r\n" +
		"Da: Ufficio Acquisti <acquisti@acme.example>\r\nInviato: lunedì 21 settembre 2026 10:15\r\n" +
		"A: Fornitore Esempio <ordini@fornitore.example>\r\nOggetto: RFQ ACME1010\r\n\r\n" +
		"Vi chiediamo quotazione per:\r\n" + testoTabDi(vecchia)
	annidata := `<table class=MsoNormalTable><tr><td>` + tabellaHTMLDi(nuova) + `</td><td>impaginazione</td></tr>
<tr><td>a</td><td>b</td></tr></table>`

	casi := []struct {
		nome, corpo, html string
		agganciate        int
	}{
		{"excel con il testo a TAB", leggiFixture(t, "excel_tab.txt"), excel, 1},
		{"excel con una cella per riga", leggiFixture(t, "excel_una_cella_per_riga.txt"), excel, 1},
		{"excel dal solo HTML", "", excel, 1},
		{"rowspan e colspan", "ACME1111\tFlangia\t10\nDado\t20\nTotale\t\t30", rowspanEColspan, 1},
		{"due tabelle, la seconda nella storia", dueTabelleTesto, dueTabelleHTML, 2},
		{"il testo non è quello dell'HTML", strings.Replace(leggiFixture(t, "excel_tab.txt"), "Flangia", "Flangie", 1), excel, 0},
		{"un'impaginazione annidata", "ACME2020\tVite\t5\r\nACME2121\tDado\t6", annidata, 1},
		{"nessun HTML", leggiFixture(t, "excel_tab.txt"), "", 0},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, tg, got := conOrigine(t, c.corpo, c.html)
			n := 0
			for _, tb := range got {
				if tb.Agganciata {
					n++
				}
			}
			if n != c.agganciate {
				t.Errorf("%d tabelle agganciate, attese %d (taglio %s)", n, c.agganciate, tg.Stato)
			}
		})
	}

	// Il testo dall'HTML è quello che Presenta mostra quando il testo semplice manca.
	if got, atteso := TestoDaHTML(excel), leggiHTML(excel).testo(); got != atteso || got == "" {
		t.Errorf("TestoDaHTML = %q, la lettura dell'HTML dà %q", got, atteso)
	}
	if !Presenta("", excel).DaHTML {
		t.Error("Presenta non legge l'HTML con il testo vuoto")
	}
	if TestoDaHTML("") != "" || TestoDaHTML("<p> \u00a0 </p>") != "" || TestoDaHTML(strings.Repeat("a", LimiteHTML+1)) != "" {
		t.Error("TestoDaHTML dà un testo dove Presenta non ne ha")
	}
}

// fMail4HTML e fMail4Righe: la forma di F-MAIL-4 (5.7.2). Una tabella HTML 6×3 (più la riga fantasma di Word,
// vuota, che la pulizia toglie), con «Q.TA» nella seconda riga e quattro righe codice / descrizione / «5»; nel
// testo una cella per riga, con i CRLF di Outlook.
const fMail4HTML = `<html><body><div class=WordSection1>
<p class=MsoNormal>Buongiorno,</p>
<p class=MsoNormal>vi giro l'elenco dei codici.</p>
<table class=MsoNormalTable>
<tr><td><p class=MsoNormal><o:p>&nbsp;</o:p></p></td><td><p class=MsoNormal><o:p>&nbsp;</o:p></p></td><td></td></tr>
<tr><td>Richiesta</td><td>ACME</td><td></td></tr>
<tr><td>Codice</td><td>Descrizione</td><td>Q.TA</td></tr>
<tr><td>PB07XX0001</td><td>Staffa</td><td>5</td></tr>
<tr><td>PB07XX0002</td><td>Piastra</td><td>5</td></tr>
<tr><td>PB07XX0003</td><td>Perno</td><td>5</td></tr>
<tr><td>PB07XX0004</td><td>Boccola</td><td>5</td></tr>
</table>
<p class=MsoNormal>Grazie</p>
</div></body></html>`

var fMail4Righe = []string{
	"Buongiorno,", "vi giro l'elenco dei codici.", "",
	"Richiesta", "ACME",
	"Codice", "Descrizione", "Q.TA",
	"PB07XX0001", "Staffa", "5",
	"PB07XX0002", "Piastra", "5",
	"PB07XX0003", "Perno", "5",
	"PB07XX0004", "Boccola", "5",
	"", "Grazie",
}

// TestOgniCellaSaLeSueRighe (A1b-11; R28 b): nella forma di F-MAIL-4 ogni cella sa le sue righe del testo, se
// coincide con loro, la sua <tr> prima della pulizia, la colonna e il percorso nel DOM.
func TestOgniCellaSaLeSueRighe(t *testing.T) {
	t.Run("una cella per riga con CRLF", func(t *testing.T) {
		corpo := strings.Join(fMail4Righe, "\r\n")
		_, tg, got := conOrigine(t, corpo, fMail4HTML)
		if tg.Stato != classificazione.StatoNessunaStoria {
			t.Fatalf("taglio %s: il corpo non ha confini", tg.Stato)
		}
		if len(got) != 1 || !got[0].Agganciata || got[0].Righe == nil || *got[0].Righe != [2]int{3, 20} {
			t.Fatalf("attesa una tabella agganciata sulle righe [3 20): %+v", got)
		}
		tb := got[0]
		if len(tb.Celle) != 18 || tb.Intestazione {
			t.Fatalf("%d celle (attese 6×3), intestazione %v", len(tb.Celle), tb.Intestazione)
		}
		riga := func(r int) *[2]int { return &[2]int{r, r + 1} }
		attese := []CellaOrigine{
			{Riga: 0, RigaHTML: 1, Cella: 0, Colonna: 0, Testo: "Richiesta", Righe: riga(3), Coincide: true},
			{Riga: 0, RigaHTML: 1, Cella: 1, Colonna: 1, Testo: "ACME", Righe: riga(4), Coincide: true},
			{Riga: 0, RigaHTML: 1, Cella: 2, Colonna: 2, Testo: ""},
			{Riga: 1, RigaHTML: 2, Cella: 0, Colonna: 0, Testo: "Codice", Righe: riga(5), Coincide: true},
			{Riga: 1, RigaHTML: 2, Cella: 1, Colonna: 1, Testo: "Descrizione", Righe: riga(6), Coincide: true},
			{Riga: 1, RigaHTML: 2, Cella: 2, Colonna: 2, Testo: "Q.TA", Righe: riga(7), Coincide: true},
		}
		for j, codice := range []string{"PB07XX0001", "PB07XX0002", "PB07XX0003", "PB07XX0004"} {
			r := 8 + 3*j
			descr := fMail4Righe[r+1]
			attese = append(attese,
				CellaOrigine{Riga: 2 + j, RigaHTML: 3 + j, Cella: 0, Colonna: 0, Testo: codice, Righe: riga(r), Coincide: true},
				CellaOrigine{Riga: 2 + j, RigaHTML: 3 + j, Cella: 1, Colonna: 1, Testo: descr, Righe: riga(r + 1), Coincide: true},
				CellaOrigine{Riga: 2 + j, RigaHTML: 3 + j, Cella: 2, Colonna: 2, Testo: "5", Righe: riga(r + 2), Coincide: true, Numero: true})
		}
		for k, a := range attese {
			a.Colspan, a.Rowspan = 1, 1
			a.Percorso = fmt.Sprintf("html/body/div[1]/table[1]/tbody[1]/tr[%d]/td[%d]", a.RigaHTML+1, a.Cella+1)
			if !reflect.DeepEqual(tb.Celle[k], a) {
				t.Errorf("cella %d:\n  %+v\natteso\n  %+v", k, tb.Celle[k], a)
			}
		}
		// L'intestazione «Q.TA» è nella seconda riga della tabella pulita, e la sua riga del testo è la 7.
		qta := tb.Celle[5]
		if r := tg.Righe[qta.Righe[0]]; corpo[r.Inizio:r.Fine] != "Q.TA" || corpo[r.Fine:r.FineTerminatore] != "\r\n" {
			t.Errorf("la riga di «Q.TA» nel corpo originale: %q", corpo[r.Inizio:r.FineTerminatore])
		}
	})

	t.Run("una riga di tabella per riga di testo, a TAB: le celle non coincidono", func(t *testing.T) {
		corpo := strings.Join([]string{
			"Buongiorno,", "vi giro l'elenco dei codici.", "",
			"Richiesta\tACME\t", "Codice\tDescrizione\tQ.TA",
			"PB07XX0001\tStaffa\t5", "PB07XX0002\tPiastra\t5", "PB07XX0003\tPerno\t5", "PB07XX0004\tBoccola\t5",
			"", "Grazie",
		}, "\r\n")
		_, _, got := conOrigine(t, corpo, fMail4HTML)
		if len(got) != 1 || !got[0].Agganciata || *got[0].Righe != [2]int{3, 9} {
			t.Fatalf("attesa una tabella agganciata sulle righe [3 9): %+v", got)
		}
		for _, cl := range got[0].Celle {
			if cl.Testo == "" {
				continue
			}
			if cl.Coincide || cl.Righe == nil || *cl.Righe != [2]int{3 + cl.Riga, 4 + cl.Riga} {
				t.Errorf("cella %q: righe %v, coincide %v; attese [%d %d) e no", cl.Testo, cl.Righe, cl.Coincide, 3+cl.Riga, 4+cl.Riga)
			}
		}
	})

	t.Run("una cella su due righe", func(t *testing.T) {
		h := `<table><tr><td>ACME1111</td><td>Staffa<br>zincata</td></tr><tr><td>ACME2222</td><td>Piastra</td></tr></table>`
		corpo := "ACME1111\nStaffa\nzincata\nACME2222\nPiastra"
		_, _, got := conOrigine(t, corpo, h)
		if len(got) != 1 || !got[0].Agganciata {
			t.Fatalf("attesa una tabella agganciata: %+v", got)
		}
		cl := got[0].Celle[1]
		if cl.Testo != "Staffa\nzincata" || cl.Righe == nil || *cl.Righe != [2]int{1, 3} || !cl.Coincide {
			t.Errorf("la cella su due righe: %+v", cl)
		}
	})

	t.Run("le colonne contano i rowspan sopra", func(t *testing.T) {
		_, _, got := conOrigine(t, "ACME1111\tFlangia\t10\nDado\t20\nTotale\t\t30", rowspanEColspan)
		if len(got) != 1 {
			t.Fatalf("attesa una tabella: %+v", got)
		}
		col := map[string]int{}
		for _, cl := range got[0].Celle {
			col[cl.Testo] = cl.Colonna
		}
		// «Dado» e «20» stanno sotto «Flangia» e «10», perché ACME1111 occupa la colonna 0 per tre righe.
		atteso := map[string]int{"ACME1111": 0, "Flangia": 1, "10": 2, "Dado": 1, "20": 2, "Totale": 1, "30": 3}
		if !reflect.DeepEqual(col, atteso) {
			t.Errorf("colonne %v, attese %v", col, atteso)
		}
	})
}

// TestUnaTabellaCheNonCombaciaNonSiAggancia (A1b-12; F-MAIL-5): una tabella che il testo non dice, una a cavallo
// del taglio, il testo marcato: nessuna cella ha righe, e la vista di oggi (Presenta) non mostra niente di
// diverso. Il corpo vuoto con solo HTML si aggancia solo sul testo ricavato dall'HTML; il ripiego a TAB non è
// una tabella HTML; con i separatori tedesco e francese la tabella si aggancia nella storia.
func TestUnaTabellaCheNonCombaciaNonSiAggancia(t *testing.T) {
	elenco := [][]string{{"Codice", "Descrizione", "Q.TA"}, {"ACME1111", "Staffa", "5"}, {"ACME2222", "Piastra", "3"}}
	elencoHTML := tabellaHTMLDi(elenco)

	soloNonAgganciate := func(t *testing.T, got []TabellaOrigine, aCavallo bool) {
		t.Helper()
		if len(got) == 0 {
			t.Fatal("nessuna tabella: l'HTML ne ha una")
		}
		for i, tb := range got {
			if tb.Agganciata || tb.ACavallo != aCavallo {
				t.Errorf("tabella %d: agganciata %v, a cavallo %v (atteso %v)", i, tb.Agganciata, tb.ACavallo, aCavallo)
			}
		}
	}

	t.Run("il testo non dice la tabella", func(t *testing.T) {
		_, _, got := conOrigine(t, "Buongiorno,\r\nvi mando l'elenco in allegato.\r\nGrazie", elencoHTML)
		soloNonAgganciate(t, got, false)
	})

	t.Run("a cavallo del taglio", func(t *testing.T) {
		// Un caso costruito apposta: le righe della tabella che vengono dopo la prima il taglio le riconosce
		// come intestazioni citate. Le parole ci sono tutte e di seguito, ma metà nella parte corrente e metà
		// nella storia: la tabella non si aggancia, si segnala.
		h := tabellaHTMLDi([][]string{{"Codice", "ACME1111"}, {"Da:", "acquisti@acme.example"}, {"Oggetto:", "offerta"}})
		corpo := "Codice ACME1111\r\nDa: acquisti@acme.example\r\nOggetto: offerta"
		_, tg, got := conOrigine(t, corpo, h)
		if tg.Stato != classificazione.StatoTagliato || tg.RigaTaglio != 1 {
			t.Fatalf("taglio %s alla riga %d, atteso tagliato alla riga 1", tg.Stato, tg.RigaTaglio)
		}
		soloNonAgganciate(t, got, true)
	})

	t.Run("testo marcato con il segno di citazione", func(t *testing.T) {
		corpo := "Va bene.\r\n\r\n> Codice\tDescrizione\tQ.TA\r\n> ACME1111\tStaffa\t5\r\n> ACME2222\tPiastra\t3"
		_, tg, got := conOrigine(t, corpo, elencoHTML)
		if tg.Regola != classificazione.RegolaTestoMarcato {
			t.Fatalf("regola %q, attesa testo_marcato", tg.Regola)
		}
		soloNonAgganciate(t, got, false)
	})

	t.Run("corpo vuoto con solo HTML", func(t *testing.T) {
		h := "<p>Buongiorno,</p>" + elencoHTML + "<p>Grazie</p>"
		// Sul corpo vuoto non c'è niente a cui agganciarsi: la tabella c'è, ma non agganciata.
		got := TabelleConOrigine("", h, classificazione.TagliaCatenaConPosizioni(""))
		soloNonAgganciate(t, got, false)
		// Sul testo ricavato dall'HTML, come fa Presenta, sì.
		testoDaHTML, _, conTesto := conOrigine(t, "", h)
		if testoDaHTML == "" || len(conTesto) != 1 || !conTesto[0].Agganciata {
			t.Errorf("sul testo dall'HTML %q la tabella non si aggancia: %+v", testoDaHTML, conTesto)
		}
	})

	t.Run("il ripiego a TAB non è una tabella HTML", func(t *testing.T) {
		corpo := testoTabDi(elenco)
		if got := TabelleConOrigine(corpo, "", classificazione.TagliaCatenaConPosizioni(corpo)); got != nil {
			t.Errorf("tabelle con l'origine senza HTML: %+v", got)
		}
		if n := len(tabelle(verifica(t, corpo, "").Blocchi, OrigineTab)); n != 1 {
			t.Errorf("la vista di oggi ha %d tabelle a TAB, attesa 1", n)
		}
	})

	for _, sep := range []struct{ nome, intestazione string }{
		{"separatore tedesco", "-----Ursprüngliche Nachricht-----\r\nVon: Einkauf ACME <einkauf@acme.example>\r\nBetreff: Anfrage ACME1111"},
		{"apertura francese", "Le 17 sept. 2026 à 09:12, Achats ACME <achats@acme.example> a écrit :"},
	} {
		t.Run(sep.nome+": la tabella si aggancia nella storia", func(t *testing.T) {
			corpo := "Merci, voir ci-dessous.\r\n\r\n" + sep.intestazione + "\r\n\r\n" + testoTabDi(elenco)
			_, tg, got := conOrigine(t, corpo, elencoHTML)
			if tg.Stato != classificazione.StatoTagliato || len(got) != 1 || !got[0].Agganciata {
				t.Fatalf("taglio %s, tabelle %+v", tg.Stato, got)
			}
			if got[0].Righe[0] < tg.RigaTaglio {
				t.Errorf("la tabella comincia alla riga %d, prima della storia (%d)", got[0].Righe[0], tg.RigaTaglio)
			}
		})
	}

	t.Run("oltre i limiti nessuna tabella", func(t *testing.T) {
		corpo := testoTabDi(elenco)
		tg := classificazione.TagliaCatenaConPosizioni(corpo)
		if got := TabelleConOrigine(corpo, elencoHTML+strings.Repeat(" ", LimiteHTML), tg); got != nil {
			t.Errorf("HTML oltre il limite: %+v", got)
		}
		lungo := strings.Repeat("ACME1111 ", LimiteTesto/9+10)
		if got := TabelleConOrigine(lungo, elencoHTML, classificazione.TagliaCatenaConPosizioni(lungo)); got != nil {
			t.Errorf("testo oltre il limite: %+v", got)
		}
	})

	t.Run("un taglio di un altro testo non aggancia niente", func(t *testing.T) {
		corpo := testoTabDi(elenco)
		got := TabelleConOrigine(corpo, elencoHTML, classificazione.TagliaCatenaConPosizioni("altro testo"))
		soloNonAgganciate(t, got, false)
	})
}
