// L1 — la presentazione del corpo delle mail: gli invarianti (piano A) e le prestazioni (piano G).
//
// Tutti i dati sono sintetici: ACME e Fornitore Esempio, codici PZ-nnn, indirizzi @acme.example e
// @fornitore.example, Safe Links con il parametro data azzerato, ID Teams finti. Le forme sono
// quelle di Outlook (Word HTML, annotazioni dei link, invito Teams), il contenuto no.
package lettura

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- strumenti

func leggiFixture(t testing.TB, nome string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatalf("fixture %s: %v", nome, err)
	}
	return string(b)
}

// verifica chiama elabora (senza la rete del recover: un panic deve vedersi) e controlla gli
// invarianti del piano A. Restituisce il Corpo per i controlli del chiamante.
func verifica(t testing.TB, testo, h string) Corpo {
	t.Helper()
	c, l := elabora(testo, h)
	if c.Originale != testo {
		t.Fatalf("Originale diverso da corpo_testo")
	}
	if l == nil {
		if len(c.Blocchi)+len(c.Storia) > 0 {
			t.Fatalf("blocchi senza sezioni")
		}
		return c
	}
	copertura(t, "Blocchi", c.Blocchi, l.fresca)
	copertura(t, "Storia", c.Storia, l.citata)
	rumore := 0
	for _, b := range append(append([]Blocco{}, c.Blocchi...), c.Storia...) {
		if b.Tipo == BloccoRumore {
			rumore++
		}
	}
	if c.NRumore < rumore {
		t.Fatalf("NRumore %d < blocchi di rumore %d", c.NRumore, rumore)
	}
	if c.SoloRumore != soloRumore(c.Blocchi) {
		t.Fatalf("SoloRumore incoerente")
	}
	c2, _ := elabora(testo, h)
	if !reflect.DeepEqual(c, c2) {
		t.Fatalf("elabora non è deterministica")
	}
	return c
}

// copertura: i blocchi coprono ogni riga della sezione, in ordine, senza buchi né sovrapposizioni;
// le parole delle celle di ogni tabella sono le parole delle righe che la tabella sostituisce.
func copertura(t testing.TB, nome string, bb []Blocco, s *sezione) {
	t.Helper()
	n := len(s.grezze)
	pos := 0
	for i, b := range bb {
		if b.Da != pos || b.A <= b.Da || b.A > n {
			t.Fatalf("%s[%d] %s copre [%d,%d), atteso inizio %d su %d righe", nome, i, b.Tipo, b.Da, b.A, pos, n)
		}
		pos = b.A
		if b.Grezzo != s.grezzo(b.Da, b.A) {
			t.Fatalf("%s[%d]: Grezzo non è il testo delle sue righe", nome, i)
		}
		switch b.Tipo {
		case BloccoTesto:
			if len(b.Pezzi) == 0 {
				t.Fatalf("%s[%d]: blocco di testo senza pezzi", nome, i)
			}
		case BloccoRumore:
			if len(b.Motivi) == 0 || b.Etichetta == "" || strings.Contains(b.Etichetta, "\n") {
				t.Fatalf("%s[%d]: rumore senza motivo o etichetta: %+v", nome, i, b)
			}
		case BloccoTabella:
			verificaTabella(t, fmt.Sprintf("%s[%d]", nome, i), b, s)
		case BloccoSeparatore:
		default:
			t.Fatalf("%s[%d]: tipo sconosciuto %q", nome, i, b.Tipo)
		}
	}
	if pos != n {
		t.Fatalf("%s: coperte %d righe su %d", nome, pos, n)
	}
}

func verificaTabella(t testing.TB, nome string, b Blocco, s *sezione) {
	t.Helper()
	tb := b.Tabella
	if tb == nil || len(tb.Righe) < 2 || tb.Colonne < 2 || tb.Colonne > maxColonne || len(tb.Righe) > maxRigheTabella {
		t.Fatalf("%s: tabella fuori dai limiti: %+v", nome, tb)
	}
	var celle []string
	troncata := false
	for r, riga := range tb.Righe {
		for _, c := range riga.Celle {
			if c.Colspan < 1 || c.Colspan > tb.Colonne || c.Rowspan < 1 || c.Rowspan > len(tb.Righe)-r {
				t.Fatalf("%s: span fuori dai limiti: %+v", nome, c)
			}
			troncata = troncata || c.Troncata
			celle = append(celle, parole(c.Testo)...)
		}
	}
	if troncata {
		return
	}
	var righe []string
	for i := b.Da; i < b.A; i++ {
		righe = append(righe, parole(s.grezze[i])...)
	}
	if strings.Join(celle, " ") != strings.Join(righe, " ") {
		t.Fatalf("%s: le celle non dicono le parole delle righe\ncelle: %q\nrighe: %q", nome, celle, righe)
	}
}

// testoDi è il testo visibile di un blocco di testo (i pezzi uno dopo l'altro).
func testoDi(b Blocco) string {
	var s strings.Builder
	for _, p := range b.Pezzi {
		s.WriteString(p.Testo)
	}
	return s.String()
}

func tipi(bb []Blocco) string {
	var s []string
	for _, b := range bb {
		s = append(s, string(b.Tipo))
	}
	return strings.Join(s, ",")
}

// ---------------------------------------------------------------- A. invarianti

func TestUnaMailSempliceEUnSoloBloccoDiTesto(t *testing.T) {
	const corpo = "Buongiorno,\r\nvi chiediamo un'offerta per il PZ-001 rev 3.\r\n\r\n\r\n\r\nCordiali saluti\r\nUfficio Acquisti ACME   \r\n"
	c := verifica(t, corpo, "")
	if len(c.Blocchi) != 1 || c.Blocchi[0].Tipo != BloccoTesto || len(c.Storia) != 0 {
		t.Fatalf("attesi un blocco di testo e niente storia, avuti %s / %s", tipi(c.Blocchi), tipi(c.Storia))
	}
	// I6: le tre righe vuote diventano una, gli spazi finali spariscono, il resto è com'è.
	const atteso = "Buongiorno,\nvi chiediamo un'offerta per il PZ-001 rev 3.\n\nCordiali saluti\nUfficio Acquisti ACME"
	if got := testoDi(c.Blocchi[0]); got != atteso {
		t.Errorf("testo:\n%q\natteso:\n%q", got, atteso)
	}
	if c.NRumore != 0 || c.SoloRumore || c.DaHTML || c.Ridotto {
		t.Errorf("segnali inattesi: %+v", c)
	}
}

func TestInvariantiSulleFixture(t *testing.T) {
	h := leggiFixture(t, "excel.html")
	for _, testo := range []string{
		leggiFixture(t, "excel_tab.txt"),
		leggiFixture(t, "excel_una_cella_per_riga.txt"),
		"",
		"   \r\n\t",
	} {
		verifica(t, testo, h)
		verifica(t, testo, "")
	}
	for _, testo := range corpusDiProva() {
		verifica(t, testo, "")
		verifica(t, testo, h)
	}
}

func TestVuotoSenzaHTMLNonHaBlocchi(t *testing.T) {
	c := verifica(t, "", "")
	if len(c.Blocchi) != 0 || c.DaHTML || c.Ridotto {
		t.Fatalf("corpo vuoto: %+v", c)
	}
}

func TestTestoOltreIlLimiteRestaOriginale(t *testing.T) {
	testo := strings.Repeat("PZ-001 ", LimiteTesto/7+10)
	c := Presenta(testo, "")
	if !c.Ridotto || len(c.Blocchi) != 0 || c.Originale != testo {
		t.Fatalf("oltre il limite: Ridotto=%v blocchi=%d", c.Ridotto, len(c.Blocchi))
	}
}

func TestPresentaEDeterministica(t *testing.T) {
	h := leggiFixture(t, "excel.html")
	testo := leggiFixture(t, "excel_tab.txt")
	a := Presenta(testo, h)
	for range 5 {
		if b := Presenta(testo, h); !reflect.DeepEqual(a, b) {
			t.Fatal("due chiamate con lo stesso ingresso danno risultati diversi")
		}
	}
}

// ---------------------------------------------------------------- A. fuzz

// corpusDiProva è il corpus sintetico dei test sul rumore e sulle catene: fa da seme al fuzz e
// passa dagli invarianti.
func corpusDiProva() []string {
	return []string{
		bannerEN, bannerIT, primoContattoEN, primoContattoIT1, primoContattoIT2, primoContattoIT3,
		teamsENClassico, teamsENNuovo, teamsIT, clausolaIT, clausolaEN, clausolaBilingue,
		ambienteConGlifo, rispostaIT, inoltroSenzaCommento,
		"Riepilogo lavorazione del PZ-001:\nDa: tornitura\nA: rettifica",
		"A\tB\nC\tD\n\nE\tF",
		"Vedi " + safeLinks("https%3A%2F%2Fwww.acme.example%2Fordini") + " e Ufficio<mailto:acquisti@acme.example>",
		"[cid:image001.png@01DA0000.00000000]\n[PZ-001]",
	}
}

func FuzzPresenta(f *testing.F) {
	h := leggiFixture(f, "excel.html")
	f.Add(leggiFixture(f, "excel_tab.txt"), h)
	f.Add(leggiFixture(f, "excel_una_cella_per_riga.txt"), h)
	f.Add("", h)
	f.Add("", "<p>Ciao</p><table><tr><td>PZ-001</td><td>10</td></tr><tr><td>PZ-002</td><td>20</td></tr></table>")
	f.Add("x", "<table><tr><td><script>alert(1)</script>PZ-001</td><td onclick=x>a</td></tr><tr><td>b</td><td>c</td></tr></table>")
	f.Add("", "<table><tr><td rowspan=0 colspan=9999>a b c</td><td>d</td></tr><tr><td>e</td><td>f</td></tr></table>")
	for _, s := range corpusDiProva() {
		f.Add(s, "")
	}
	f.Fuzz(func(t *testing.T, testo, h string) {
		verifica(t, testo, h)
	})
}

// ---------------------------------------------------------------- G. prestazioni

// wordHTMLGrande costruisce un messaggio di Outlook da circa 200 KB di HTML (Word HTML: paragrafi
// MsoNormal, una tabella Excel da venti righe ogni otto paragrafi, una quindicina in tutto) e il suo
// testo semplice, con le righe delle tabelle separate da TAB come le scrive Outlook.
func wordHTMLGrande() (testo, h string) {
	var ht, tx strings.Builder
	ht.WriteString(`<html><head><style>p.MsoNormal{margin:0cm}</style><!--[if gte mso 9]><xml><o:shapedefaults/></xml><![endif]--></head><body><div class=WordSection1>`)
	riga := 0
	for ht.Len() < 200_000 {
		for range 8 {
			riga++
			p := fmt.Sprintf("Riga %d: confermiamo le quantità del PZ-%03d secondo il disegno allegato.", riga, riga%1000)
			fmt.Fprintf(&ht, `<p class=MsoNormal><span style='color:#1F3864'>%s<o:p></o:p></span></p>`+"\n", p)
			tx.WriteString(p + "\r\n")
		}
		ht.WriteString(`<table class=MsoNormalTable border=0 cellspacing=0 cellpadding=0>`)
		for r := range 20 {
			ht.WriteString(`<tr>`)
			celle := []string{fmt.Sprintf("PZ-%03d", r), "Flangia forata", fmt.Sprintf("%d", 10*r+5)}
			for k, c := range celle {
				al := ""
				if k == 2 {
					al = ` align=right style='text-align:right'`
				}
				fmt.Fprintf(&ht, `<td nowrap valign=bottom style='border:solid windowtext 1.0pt;padding:0cm 5.4pt'><p class=MsoNormal%s><span style='color:black'>%s<o:p></o:p></span></p></td>`, al, c)
			}
			ht.WriteString("</tr>\n")
			tx.WriteString(strings.Join(celle, "\t") + "\r\n")
		}
		ht.WriteString("</table>\n<p class=MsoNormal><o:p>&nbsp;</o:p></p>\n")
		tx.WriteString("\r\n")
	}
	ht.WriteString(`</div></body></html>`)
	return tx.String(), ht.String()
}

func BenchmarkPresenta(b *testing.B) {
	testo, h := wordHTMLGrande()
	b.SetBytes(int64(len(h)))
	c := Presenta(testo, h)
	tab := 0
	for _, bl := range c.Blocchi {
		if bl.Tipo == BloccoTabella && bl.Tabella.Origine == OrigineHTML {
			tab++
		}
	}
	if tab == 0 {
		b.Fatal("nessuna tabella ancorata: il banco non misura il caso vero")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		Presenta(testo, h)
	}
}

// BenchmarkPresentaCasiEstremi: un mega di testo costruito per essere lento. Ognuno di questi casi
// era quadratico in una prima versione (secondi, non millisecondi): il banco resta per vederlo se
// torna.
func BenchmarkPresentaCasiEstremi(b *testing.B) {
	righe := func(r string, n int) string { return strings.Repeat(r+"\n", n) }
	tab := "<table>" + strings.Repeat("<tr><td>PZ-001</td><td>PZ-001</td></tr>", 3) + "</table>"
	for _, caso := range []struct{ nome, testo, html string }{
		{"firme iPhone", righe("Inviato da iPhone", 50_000), ""},
		{"righe Teams", righe("Microsoft Teams", 60_000), ""},
		{"glifo e ambiente", righe("P\nPensate all'ambiente prima di stampare", 20_000), ""},
		{"righe TAB", righe("a\tb", 200_000), ""},
		{"parole uguali", righe("PZ-001", 140_000), strings.Repeat(tab, maxTabelle)},
		{"HTML di paragrafi", "", strings.Repeat("<p>x", 300_000)},
	} {
		b.Run(caso.nome, func(b *testing.B) {
			for b.Loop() {
				Presenta(caso.testo, caso.html)
			}
		})
	}
}

func TestIlBancoDelleProveHaLaMisuraGiusta(t *testing.T) {
	testo, h := wordHTMLGrande()
	if len(h) < 200_000 || len(h) > 230_000 {
		t.Fatalf("HTML del banco: %d byte", len(h))
	}
	c := verifica(t, testo, h)
	tab := 0
	for _, b := range c.Blocchi {
		if b.Tipo == BloccoTabella && b.Tabella.Origine == OrigineHTML {
			tab++
		}
	}
	if tab < 10 {
		t.Fatalf("tabelle ancorate: %d", tab)
	}
}
