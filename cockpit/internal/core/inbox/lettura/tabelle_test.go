// L1 — le tabelle: dal corpo HTML ancorate nel testo (piano D) e il fallback TAB (piano E).
package lettura

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"unicode/utf8"
)

// tabellaHTMLDi e testoTabDi costruiscono la stessa tabella nelle due forme del messaggio.
func tabellaHTMLDi(righe [][]string) string {
	var b strings.Builder
	b.WriteString("<table class=MsoNormalTable>")
	for _, r := range righe {
		b.WriteString("<tr>")
		for _, c := range r {
			b.WriteString("<td><p class=MsoNormal>" + html.EscapeString(c) + "<o:p></o:p></p></td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>")
	return b.String()
}

func testoTabDi(righe [][]string) string {
	s := make([]string, len(righe))
	for i, r := range righe {
		s[i] = strings.Join(r, "\t")
	}
	return strings.Join(s, "\r\n")
}

func tabelle(bb []Blocco, origine string) []*Tabella {
	var out []*Tabella
	for _, b := range bb {
		if b.Tipo == BloccoTabella && (origine == "" || b.Tabella.Origine == origine) {
			out = append(out, b.Tabella)
		}
	}
	return out
}

func celle(t *Tabella) [][]string {
	var out [][]string
	for _, r := range t.Righe {
		var s []string
		for _, c := range r.Celle {
			s = append(s, c.Testo)
		}
		out = append(out, s)
	}
	return out
}

// ---------------------------------------------------------------- D. excel.html

// controllaExcel: [testo «Buongiorno,\nvi chiediamo quotazione per:»], [tabella 3×3 con
// intestazione, colonna 3 numerica, una cella vuota], [testo «Grazie»].
func controllaExcel(t *testing.T, c Corpo) {
	t.Helper()
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoTabella, BloccoTesto)
	if got := testoDi(c.Blocchi[0]); got != "Buongiorno,\nvi chiediamo quotazione per:" {
		t.Errorf("testo prima: %q", got)
	}
	if got := testoDi(c.Blocchi[2]); got != "Grazie" {
		t.Errorf("testo dopo: %q", got)
	}
	tb := c.Blocchi[1].Tabella
	if tb.Origine != OrigineHTML || tb.Colonne != 3 || len(tb.Righe) != 3 || !tb.Intestazione {
		t.Fatalf("tabella: origine %s, %d colonne, %d righe, intestazione %v", tb.Origine, tb.Colonne, len(tb.Righe), tb.Intestazione)
	}
	want := [][]string{{"Codice", "Descrizione", "Q.tà"}, {"PZ-001", "Flangia", "100"}, {"PZ-002", "", "250"}}
	if got := celle(tb); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("celle %q, attese %q", got, want)
	}
	for r := 1; r < 3; r++ {
		if !tb.Righe[r].Celle[2].Numero || tb.Righe[r].Celle[0].Numero {
			t.Errorf("riga %d: la colonna 3 deve essere numerica e la 1 no", r)
		}
	}
	for _, r := range tb.Righe {
		for _, cl := range r.Celle {
			if cl.Colspan != 1 || cl.Rowspan != 1 || cl.Troncata {
				t.Errorf("cella %+v", cl)
			}
		}
	}
}

func TestExcelConIlTestoATab(t *testing.T) {
	c := verifica(t, leggiFixture(t, "excel_tab.txt"), leggiFixture(t, "excel.html"))
	controllaExcel(t, c)
}

func TestExcelConUnaCellaPerRiga(t *testing.T) {
	c := verifica(t, leggiFixture(t, "excel_una_cella_per_riga.txt"), leggiFixture(t, "excel.html"))
	controllaExcel(t, c)
}

func TestExcelSoloHTML(t *testing.T) {
	c := verifica(t, "", leggiFixture(t, "excel.html"))
	if !c.DaHTML {
		t.Fatal("DaHTML falso con il testo vuoto")
	}
	controllaExcel(t, c)
}

func TestExcelSenzaHTMLDiventaTabellaTab(t *testing.T) {
	c := verifica(t, leggiFixture(t, "excel_tab.txt"), "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoTabella, BloccoTesto)
	if tb := c.Blocchi[1].Tabella; tb.Origine != OrigineTab || tb.Intestazione || tb.Colonne != 3 {
		t.Errorf("tabella tab: %+v", tb)
	}
}

// ---------------------------------------------------------------- D. i casi al bordo

func TestColspanERowspanLimitati(t *testing.T) {
	const h = `<table>
<tr><td rowspan=99>PZ-010</td><td>Flangia</td><td>10</td></tr>
<tr><td>Dado</td><td>20</td></tr>
<tr><td colspan=2>Totale</td><td colspan=abc>30</td></tr>
</table>`
	c := verifica(t, "PZ-010\tFlangia\t10\nDado\t20\nTotale\t\t30", h)
	tt := tabelle(c.Blocchi, OrigineHTML)
	if len(tt) != 1 {
		t.Fatalf("tabelle html: %d (%s)", len(tt), tipi(c.Blocchi))
	}
	tb := tt[0]
	if tb.Colonne != 3 || tb.Righe[0].Celle[0].Rowspan != 3 || tb.Righe[2].Celle[0].Colspan != 2 || tb.Righe[2].Celle[1].Colspan != 1 {
		t.Errorf("span: %+v", tb)
	}
}

func TestLaFirmaAImpaginazioneAnnidataNonETabella(t *testing.T) {
	const h = `<p class=MsoNormal>Grazie per la collaborazione.</p>
<table class=MsoNormalTable><tr>
<td><p class=MsoNormal><img src="cid:image001.png@01DA0000.00000000" alt="logo"></p></td>
<td><table><tr><td><p>Fornitore Esempio S.r.l.</p></td></tr><tr><td><p>Ufficio Tecnico</p></td></tr><tr><td><p>ordini@fornitore.example</p></td></tr></table></td>
</tr></table>`
	const testo = "Grazie per la collaborazione.\n\n[cid:image001.png@01DA0000.00000000]\tFornitore Esempio S.r.l.\nUfficio Tecnico\nordini@fornitore.example"
	c := verifica(t, testo, h)
	if n := len(tabelle(c.Blocchi, "")); n != 0 {
		t.Errorf("la firma è diventata una tabella (%s)", tipi(c.Blocchi))
	}
}

func TestLaTabellaDelPrimoContattoNonETabella(t *testing.T) {
	for nome, h := range map[string]string{
		"una riga": `<table><tr><td></td><td>You don't often get email from ordini@fornitore.example. <a href="https://aka.ms/LearnAboutSenderIdentification">Learn why this is important</a></td></tr></table>`,
		"due righe": `<table><tr><td>Avviso</td><td>You don't often get email from ordini@fornitore.example.</td></tr>` +
			`<tr><td>Info</td><td><a href="https://aka.ms/LearnAboutSenderIdentification">Learn why this is important</a></td></tr></table>`,
	} {
		t.Run(nome, func(t *testing.T) {
			testo := primoContattoEN
			if nome == "due righe" {
				testo = "Avviso\tYou don't often get email from ordini@fornitore.example.\nInfo\tLearn why this is important<https://aka.ms/LearnAboutSenderIdentification>\n\nBuongiorno, alleghiamo l'offerta per il PZ-001."
			}
			doc := leggiHTML(h)
			if len(doc.tabelle) != 0 {
				t.Errorf("il primo contatto è una tabella di dati")
			}
			c := verifica(t, testo, h)
			soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
		})
	}
}

func TestTestoDiversoDallHTMLNienteTabellaHTML(t *testing.T) {
	testo := strings.Replace(leggiFixture(t, "excel_tab.txt"), "PZ-002", "PZ-009", 1)
	c := verifica(t, testo, leggiFixture(t, "excel.html"))
	if n := len(tabelle(c.Blocchi, OrigineHTML)); n != 0 {
		t.Errorf("una tabella HTML si è ancorata su un testo diverso")
	}
}

func TestDueTabelleLaSecondaCitataVaInStoria(t *testing.T) {
	nuova := [][]string{{"PZ-020", "Vite", "5"}, {"PZ-021", "Dado", "6"}}
	vecchia := [][]string{{"PZ-010", "Flangia", "100"}, {"PZ-011", "Rondella", "200"}}
	h := "<p class=MsoNormal>Ecco la nuova lista:</p>" + tabellaHTMLDi(nuova) +
		`<div style="border:none;border-top:solid #E1E1E1 1.0pt"><p class=MsoNormal><b>Da:</b> Ufficio Acquisti</p></div>` +
		"<p class=MsoNormal>Vi chiediamo quotazione per:</p>" + tabellaHTMLDi(vecchia)
	testo := "Ecco la nuova lista:\r\n" + testoTabDi(nuova) + "\r\n\r\n________________________________\r\n" +
		"Da: Ufficio Acquisti <acquisti@acme.example>\r\nInviato: lunedì 21 settembre 2026 10:15\r\n" +
		"A: Fornitore Esempio <ordini@fornitore.example>\r\nOggetto: RFQ PZ-010\r\n\r\n" +
		"Vi chiediamo quotazione per:\r\n" + testoTabDi(vecchia)
	c := verifica(t, testo, h)
	fresche, citate := tabelle(c.Blocchi, OrigineHTML), tabelle(c.Storia, OrigineHTML)
	if len(fresche) != 1 || len(citate) != 1 {
		t.Fatalf("tabelle html: %d fresche, %d citate", len(fresche), len(citate))
	}
	if fresche[0].Righe[0].Celle[0].Testo != "PZ-020" || citate[0].Righe[0].Celle[0].Testo != "PZ-010" {
		t.Errorf("tabelle al posto sbagliato")
	}
}

func TestTroppeRigheOColonneNonETabella(t *testing.T) {
	grande := func(righe, colonne int) [][]string {
		var out [][]string
		for r := range righe {
			var riga []string
			for k := range colonne {
				riga = append(riga, fmt.Sprintf("PZ-%d-%d", r, k))
			}
			out = append(out, riga)
		}
		return out
	}
	for nome, dati := range map[string][][]string{"250 righe": grande(250, 3), "40 colonne": grande(3, 40)} {
		t.Run(nome, func(t *testing.T) {
			c := verifica(t, testoTabDi(dati), tabellaHTMLDi(dati))
			if n := len(tabelle(c.Blocchi, "")); n != 0 {
				t.Errorf("%s: %d tabelle", nome, n)
			}
		})
	}
}

func TestLaCellaLungaSiTronca(t *testing.T) {
	lunga := strings.TrimSpace(strings.Repeat("flangia ", 75)) + " PZ-001" // 606 rune
	dati := [][]string{{"PZ-001", lunga}, {"PZ-002", "Dado"}}
	c := verifica(t, testoTabDi(dati), tabellaHTMLDi(dati))
	tt := tabelle(c.Blocchi, OrigineHTML)
	if len(tt) != 1 {
		t.Fatalf("tabelle html: %d", len(tt))
	}
	cl := tt[0].Righe[0].Celle[1]
	if !cl.Troncata || utf8.RuneCountInString(cl.Testo) != 500 || !strings.HasPrefix(lunga, cl.Testo) {
		t.Errorf("cella lunga: troncata %v, %d rune", cl.Troncata, utf8.RuneCountInString(cl.Testo))
	}
}

func TestHTMLOltreIlLimiteNonSiLegge(t *testing.T) {
	h := strings.Replace(leggiFixture(t, "excel.html"), "</body>", "<p>"+strings.Repeat("x", LimiteHTML)+"</p></body>", 1)
	c := verifica(t, leggiFixture(t, "excel_una_cella_per_riga.txt"), h)
	if n := len(tabelle(c.Blocchi, "")); n != 0 {
		t.Errorf("HTML oltre il limite letto lo stesso")
	}
}

func TestHTMLMalformatoNonRompeNiente(t *testing.T) {
	profondo := strings.Repeat("<div>", 600) + "PZ-001 profondo" + strings.Repeat("</div>", 600)
	for _, h := range []string{
		"<table><tr><td>PZ-001<td>Flangia<tr><td>PZ-002<td>Dado",
		"<<<>>><table<tr",
		"<![if !supportLists]><td><td></table></table>",
		"<table><caption>x</caption><colgroup><col></colgroup><tbody><tr><th>a</th></tr></tbody></table>",
		"<svg><table><tr><td>a</td></tr></table></svg><math><mi>",
		"<table><tr><td rowspan=-5 colspan=99999999>a b</td><td>c</td></tr><tr><td>d</td><td>e</td></tr></table>",
		profondo,
		"\x00<table>\xff\xfe<tr><td>\x00</td></tr>",
	} {
		verifica(t, "PZ-001\tFlangia\nPZ-002\tDado", h)
		verifica(t, "", h)
	}
	// Le chiusure implicite dell'HTML5 sono HTML valido: la tabella si ancora.
	c := verifica(t, "PZ-001\tFlangia\nPZ-002\tDado", "<table><tr><td>PZ-001<td>Flangia<tr><td>PZ-002<td>Dado")
	if len(tabelle(c.Blocchi, OrigineHTML)) != 1 {
		t.Errorf("tabella con chiusure implicite non ancorata: %s", tipi(c.Blocchi))
	}
	// Oltre 512 livelli il parser rifiuta il documento: il testo viene dai token, senza tabelle.
	c = verifica(t, "", "<script>alert(1)</script>"+profondo)
	if !c.DaHTML || len(c.Blocchi) != 1 || testoDi(c.Blocchi[0]) != "PZ-001 profondo" {
		t.Errorf("ripiego sui token: %+v", c)
	}
}

func TestHTMLOstileNonArrivaNelleCelle(t *testing.T) {
	const h = `<table><tr><td><script>alert(1)</script>PZ-001</td><td onclick="alert(2)">Flangia<style>td{color:red}</style></td></tr>` +
		`<tr><td>PZ-002</td><td><img src=x onerror=alert(3)><span style="display:none">nascosto</span>Dado</td></tr></table>`
	for _, testo := range []string{"PZ-001\tFlangia\nPZ-002\tDado", ""} {
		c := verifica(t, testo, h)
		tt := tabelle(c.Blocchi, OrigineHTML)
		if len(tt) != 1 {
			t.Fatalf("tabelle html: %d (%s)", len(tt), tipi(c.Blocchi))
		}
		for _, r := range tt[0].Righe {
			for _, cl := range r.Celle {
				if strings.ContainsAny(cl.Testo, "<>{}") || strings.Contains(cl.Testo, "alert") || strings.Contains(cl.Testo, "nascosto") {
					t.Errorf("cella con contenuto ostile: %q", cl.Testo)
				}
			}
		}
	}
}

func TestLaTabellaNascostaNonETabella(t *testing.T) {
	h := strings.Replace(leggiFixture(t, "excel.html"), `<table class="MsoNormalTable"`, `<table style="display:none" class="MsoNormalTable"`, 1)
	c := verifica(t, leggiFixture(t, "excel_una_cella_per_riga.txt"), h)
	if n := len(tabelle(c.Blocchi, "")); n != 0 {
		t.Errorf("tabella nascosta mostrata")
	}
}

func TestGliStiliSiLeggonoSenzaBadareAMaiuscoleESpazi(t *testing.T) {
	const h = `<table><tr><th>Codice</th><td style="FONT-WEIGHT: 700">Stato</td></tr>
<tr><td>PZ-001</td><td style="color:red; text-align : RIGHT">n.d.</td></tr>
<tr style="DISPLAY : None !important"><td>PZ-XXX</td><td>riga nascosta</td></tr>
<tr><td>PZ-002</td><td><span style='mso-hide:all'>nascosto </span>pronto</td></tr></table>`
	c := verifica(t, "Codice\tStato\nPZ-001\tn.d.\nPZ-002\tpronto", h)
	tt := tabelle(c.Blocchi, OrigineHTML)
	if len(tt) != 1 {
		t.Fatalf("tabelle html: %d (%s)", len(tt), tipi(c.Blocchi))
	}
	tb := tt[0]
	want := [][]string{{"Codice", "Stato"}, {"PZ-001", "n.d."}, {"PZ-002", "pronto"}}
	if fmt.Sprint(celle(tb)) != fmt.Sprint(want) {
		t.Errorf("celle %q, attese %q", celle(tb), want)
	}
	if !tb.Intestazione {
		t.Error("<th> e font-weight:700 in riga 0: è l'intestazione")
	}
	if !tb.Righe[1].Celle[1].Numero || tb.Righe[2].Celle[1].Numero {
		t.Error("text-align:right fa una cella da allineare a destra, il testo da solo no")
	}
}

func TestSoloHTMLParagrafiDiAltriClient(t *testing.T) {
	// Un <p> che non è di Word ha il suo margine: fra un paragrafo e l'altro c'è una riga vuota, e
	// il banner in cima resta un paragrafo a sé invece di portarsi dietro il testo.
	const h = `<p>This email originated from outside of the organization.</p><p>Buongiorno,</p><p>vi mandiamo il PZ-001.</p>`
	c := verifica(t, "", h)
	soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
	if got := testoDi(c.Blocchi[1]); got != "Buongiorno,\n\nvi mandiamo il PZ-001." {
		t.Errorf("testo: %q", got)
	}
}

// ---------------------------------------------------------------- E. fallback TAB

func TestTabTreRigheConTabFinale(t *testing.T) {
	c := verifica(t, "Codice\tDescrizione\tQ.tà\t\nPZ-001\tFlangia\t100\t\nPZ-002\tDado\t1.250,50\t", "")
	soloBlocchi(t, c.Blocchi, BloccoTabella)
	tb := c.Blocchi[0].Tabella
	if tb.Origine != OrigineTab || tb.Intestazione || tb.Colonne != 3 || len(tb.Righe) != 3 {
		t.Fatalf("tabella: %+v", tb)
	}
	if !tb.Righe[1].Celle[2].Numero || !tb.Righe[2].Celle[2].Numero || tb.Righe[0].Celle[2].Numero || tb.Righe[1].Celle[0].Numero {
		t.Errorf("numeri: %+v", tb.Righe)
	}
}

func TestTabUnaRigaSolaNo(t *testing.T) {
	c := verifica(t, "Vi confermo:\nPZ-001\tFlangia\t100\nGrazie", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestTabRigheIndentateNo(t *testing.T) {
	c := verifica(t, "Elenco:\n\tvoce uno\n\tvoce due\n\tvoce tre", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestTabColonneTroppoDiverseNo(t *testing.T) {
	c := verifica(t, "a\tb\nc\td\te\tf\tg\nh\ti", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestTabRigaVuotaSingolaAmmessa(t *testing.T) {
	c := verifica(t, "PZ-001\tFlangia\n\nPZ-002\tDado\nPZ-003\tVite", "")
	soloBlocchi(t, c.Blocchi, BloccoTabella)
	if n := len(c.Blocchi[0].Tabella.Righe); n != 3 {
		t.Errorf("righe: %d", n)
	}
	c = verifica(t, "PZ-001\tFlangia\n\n\nPZ-002\tDado", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestTabRigheCorteCompletate(t *testing.T) {
	c := verifica(t, "PZ-001\tFlangia\t100\nPZ-002\tDado", "")
	soloBlocchi(t, c.Blocchi, BloccoTabella)
	if r := c.Blocchi[0].Tabella.Righe[1]; len(r.Celle) != 3 || r.Celle[2].Testo != "" {
		t.Errorf("riga corta non completata: %+v", r)
	}
}
