package lettura

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// LE TABELLE DEL CORPO HTML
//
// Una tabella incollata da Excel in Outlook arriva nel corpo HTML come tabella vera (Word HTML:
// «MsoNormalTable», celle vuote «<o:p>&nbsp;</o:p>», commenti condizionali, la riga fantasma
// «supportMisalignedColumns») e nel testo semplice come righe con i TAB, o una cella per riga. Qui
// si legge l'HTML solo per tirarne fuori le tabelle di DATI, come testo: nessun marcatore, nessun
// attributo, nessuno stile esce da questo file. Il testo resta la spina dorsale — una tabella si
// mostra solo se le sue parole si ritrovano, identiche e nello stesso ordine, nel testo (ancoraggio.go).
//
// Il parser è quello di golang.org/x/net/html (l'algoritmo HTML5, tollerante per definizione):
// encoding/xml si ferma al primo «<![if …]>» di Word. Il parser rifiuta i documenti annidati oltre
// 512 elementi: allora niente tabelle, e il testo resta testo.

// Gli elementi il cui contenuto non è testo della mail.
var elementiSaltati = map[string]bool{
	"script": true, "style": true, "head": true, "title": true, "xml": true,
	"noscript": true, "template": true, "iframe": true, "noembed": true, "noframes": true,
}

// Gli elementi che vanno a capo prima e dopo.
var elementiBlocco = map[atom.Atom]bool{
	atom.Address: true, atom.Article: true, atom.Aside: true, atom.Blockquote: true, atom.Caption: true,
	atom.Center: true, atom.Dd: true, atom.Details: true, atom.Dialog: true, atom.Div: true, atom.Dl: true,
	atom.Dt: true, atom.Fieldset: true, atom.Figcaption: true, atom.Figure: true, atom.Footer: true,
	atom.Form: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Header: true, atom.Hgroup: true, atom.Li: true, atom.Main: true, atom.Nav: true, atom.Ol: true,
	atom.P: true, atom.Pre: true, atom.Section: true, atom.Summary: true, atom.Table: true, atom.Tbody: true,
	atom.Td: true, atom.Tfoot: true, atom.Th: true, atom.Thead: true, atom.Tr: true, atom.Ul: true,
}

// Gli elementi a testo grezzo del tokenizzatore: il loro contenuto è un unico pezzo di testo che
// non va mostrato (serve solo al ripiego senza albero).
var elementiGrezzi = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Title: true, atom.Noscript: true,
	atom.Iframe: true, atom.Noembed: true, atom.Noframes: true,
}

var reNumero = regexp.MustCompile(`(?i)^[-+]?(?:\d{1,3}(?:[.\s]\d{3})+|\d+)(?:[.,]\d+)?\s?(?:%|€|eur|pz|pcs|kg|mm)?$`)

// numerico: il testo della cella è un numero (con migliaia, decimali e un'unità semplice).
func numerico(s string) bool {
	s = strings.TrimSpace(perConfronto(s))
	return s != "" && len(s) <= 64 && reNumero.MatchString(s)
}

// documentoHTML è il corpo HTML letto una volta sola.
type documentoHTML struct {
	sorgente string
	radice   *html.Node // nil se il parser ha rifiutato il documento
	tabelle  []*tabellaHTML
	dati     map[*html.Node]*tabellaHTML
}

// tabellaHTML è una tabella di dati: le righe vuote già tolte, i rowspan già ricontati.
type tabellaHTML struct {
	righe        [][]cellaHTML
	colonne      int
	intestazione bool
}

type cellaHTML struct {
	testo            string // intero: il confronto con il testo usa questo
	colspan, rowspan int
	th               bool
	grassetto        bool // tutto il testo della cella è in grassetto
	destra           bool // allineata a destra
}

// contieneTabella: c'è almeno un «<table» nell'HTML? Se no il parser non serve.
func contieneTabella(h string) bool {
	for i := 0; ; {
		k := strings.IndexByte(h[i:], '<')
		if k < 0 {
			return false
		}
		i += k + 1
		if i+5 <= len(h) && strings.EqualFold(h[i:i+5], "table") {
			return true
		}
	}
}

// leggiHTML legge il corpo HTML e ne raccoglie le tabelle di dati, in ordine di documento.
func leggiHTML(h string) *documentoHTML {
	d := &documentoHTML{sorgente: h, dati: map[*html.Node]*tabellaHTML{}}
	radice, err := html.Parse(strings.NewReader(h))
	if err != nil {
		return d
	}
	d.radice = radice
	visita(radice, func(n *html.Node) bool {
		switch n.Type {
		case html.DocumentNode:
			return true
		case html.ElementNode:
		default:
			return false
		}
		if saltato(n) {
			return false // (7): né lei né un antenato nascosti
		}
		if n.DataAtom == atom.Table && len(d.tabelle) < maxTabelle {
			if t := esaminaTabella(n); t != nil {
				d.tabelle = append(d.tabelle, t)
				d.dati[n] = t
				return false // (1): una tabella di dati non ha tabelle dentro, e le sue celle sono già lette
			}
		}
		return true
	}, nil)
	return d
}

// visita percorre il sottoalbero di n in ordine di documento, senza ricorsione. entra dice se
// scendere nei figli; esci (se c'è) si chiama dopo i figli, solo per i nodi in cui si è scesi.
func visita(n *html.Node, entra func(*html.Node) bool, esci func(*html.Node)) {
	type passo struct {
		n      *html.Node
		uscita bool
	}
	pila := []passo{{n: n}}
	for len(pila) > 0 {
		p := pila[len(pila)-1]
		pila = pila[:len(pila)-1]
		if p.uscita {
			esci(p.n)
			continue
		}
		if !entra(p.n) {
			continue
		}
		if esci != nil {
			pila = append(pila, passo{n: p.n, uscita: true})
		}
		for c := p.n.LastChild; c != nil; c = c.PrevSibling {
			pila = append(pila, passo{n: c})
		}
	}
}

// saltato: un elemento il cui contenuto non si legge, perché non è testo o perché è nascosto.
func saltato(n *html.Node) bool {
	return elementiSaltati[n.Data] || aspettoDi(n).nascosto
}

// aspetto è quello che conta dell'aspetto di un elemento, letto una volta sola: l'attributo style
// di Word è lungo, e rileggerlo per ogni domanda era la voce più cara dopo il parser.
type aspetto struct {
	nascosto  bool // display:none, mso-hide:all
	destra    bool // align=right, text-align:right
	grassetto bool // <b>, <strong>, <th>, font-weight:bold|600–900
}

func aspettoDi(n *html.Node) aspetto {
	var a aspetto
	switch n.DataAtom {
	case atom.B, atom.Strong, atom.Th:
		a.grassetto = true
	}
	for _, at := range n.Attr {
		if at.Namespace != "" {
			continue
		}
		switch at.Key {
		case "align":
			a.destra = a.destra || strings.EqualFold(strings.TrimSpace(at.Val), "right")
		case "style":
			a.leggiStile(at.Val)
		}
	}
	return a
}

// leggiStile scorre le dichiarazioni «nome: valore» dell'attributo style, senza badare a
// maiuscole e spazi («DISPLAY : none !important» è display:none) e senza copiarlo.
func (a *aspetto) leggiStile(stile string) {
	for stile != "" {
		var dich string
		dich, stile, _ = strings.Cut(stile, ";")
		nome, valore, ok := strings.Cut(dich, ":")
		if !ok {
			continue
		}
		nome, valore = strings.TrimSpace(nome), strings.TrimSpace(valore)
		switch {
		case strings.EqualFold(nome, "display"):
			a.nascosto = a.nascosto || iniziaFold(valore, "none")
		case strings.EqualFold(nome, "mso-hide"):
			a.nascosto = a.nascosto || iniziaFold(valore, "all")
		case strings.EqualFold(nome, "text-align"):
			a.destra = a.destra || iniziaFold(valore, "right")
		case strings.EqualFold(nome, "font-weight"):
			a.grassetto = a.grassetto || iniziaFold(valore, "bold") ||
				(len(valore) >= 3 && valore[0] >= '6' && valore[0] <= '9' && valore[1:3] == "00")
		}
	}
}

func iniziaFold(s, prefisso string) bool {
	return len(s) >= len(prefisso) && strings.EqualFold(s[:len(prefisso)], prefisso)
}

func attr(n *html.Node, chiave string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == chiave {
			return a.Val
		}
	}
	return ""
}

// numeroAttr legge un attributo numerico come fanno i browser: le cifre iniziali, il resto no.
// Senza cifre vale predefinito.
func numeroAttr(n *html.Node, chiave string, predefinito int) int {
	v := strings.TrimSpace(attr(n, chiave))
	x, cifre := 0, 0
	for cifre < len(v) && cifre < 6 && v[cifre] >= '0' && v[cifre] <= '9' {
		x = x*10 + int(v[cifre]-'0')
		cifre++
	}
	if cifre == 0 {
		return predefinito
	}
	return x
}

// ---------------------------------------------------------------- la tabella di dati

// esaminaTabella applica i criteri della tabella di dati; nil se non lo è.
//
//	(1) nessuna cella contiene una <table>: l'esterna è impaginazione, le interne si esaminano;
//	(2) tolte le righe tutte vuote, 2–200 righe;
//	(3) colonne effettive (la somma più grande dei colspan di una riga) 2–30;
//	(4) almeno 2 righe con almeno 2 celle non vuote;
//	(5) testo totale ≤ 64 KiB;
//	(6) il testo non è un avviso di posta esterna, un primo contatto, un invito Teams;
//	(7) né lei né un antenato nascosti (lo garantisce la visita, che non scende nei nascosti).
func esaminaTabella(t *html.Node) *tabellaHTML {
	var righe [][]cellaHTML
	var collegamenti []string
	totale, piene := 0, 0
	for _, tr := range righeDi(t) {
		var riga []cellaHTML
		vuota := true
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || (c.DataAtom != atom.Td && c.DataAtom != atom.Th) || saltato(c) {
				continue
			}
			cella, annidata := leggiCella(c, &collegamenti, maxTestoTabella-totale)
			if annidata {
				return nil
			}
			totale += len(cella.testo)
			if totale > maxTestoTabella {
				return nil
			}
			if cella.testo != "" {
				vuota = false
			}
			riga = append(riga, cella)
		}
		if !vuota {
			if piene++; piene > maxRigheTabella {
				return nil
			}
		}
		righe = append(righe, riga)
	}
	righe = togliRigheVuote(righe)
	if len(righe) < 2 || len(righe) > maxRigheTabella {
		return nil
	}
	colonne := 0
	for _, r := range righe {
		somma := 0
		for _, c := range r {
			somma += c.colspan
		}
		colonne = max(colonne, somma)
	}
	if colonne < 2 || colonne > maxColonne {
		return nil
	}
	conDue := 0
	var tutto strings.Builder
	for _, r := range righe {
		n := 0
		for _, c := range r {
			if c.testo != "" {
				n++
				tutto.WriteString(c.testo)
				tutto.WriteByte('\n')
			}
		}
		if n >= 2 {
			conDue++
		}
	}
	if conDue < 2 {
		return nil
	}
	for _, u := range collegamenti {
		tutto.WriteString(u)
		tutto.WriteByte('\n')
	}
	if èRumoreDiTabella(tutto.String()) {
		return nil
	}
	return &tabellaHTML{righe: righe, colonne: colonne, intestazione: intestazione(righe)}
}

// righeDi sono le righe della tabella t, e solo sue: quelle di una tabella annidata stanno dentro
// una cella e qui non si vedono.
func righeDi(t *html.Node) []*html.Node {
	var out []*html.Node
	for c := t.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || saltato(c) {
			continue
		}
		switch c.DataAtom {
		case atom.Tr:
			out = append(out, c)
		case atom.Thead, atom.Tbody, atom.Tfoot:
			for r := c.FirstChild; r != nil; r = r.NextSibling {
				if r.Type == html.ElementNode && r.DataAtom == atom.Tr && !saltato(r) {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

// leggiCella legge il testo di una cella: spazi compressi, a capo sui <br> e sui confini di
// paragrafo, niente dai nascosti e dagli script, niente dalle immagini. annidata è vero se la
// cella contiene una tabella: allora la tabella che la contiene non è di dati, e il resto non serve.
func leggiCella(td *html.Node, collegamenti *[]string, limite int) (c cellaHTML, annidata bool) {
	c.th = td.DataAtom == atom.Th
	c.colspan = min(max(numeroAttr(td, "colspan", 1), 1), 1000)
	c.rowspan = min(numeroAttr(td, "rowspan", 1), 65534) // 0: fino in fondo
	w := scrittore{limite: max(limite, 0) + 1}
	grassetto := 0    // quanti antenati (nella cella) sono in grassetto
	var aperti []bool // per ogni elemento aperto, se ha contato in grassetto
	conTesto, tuttoGrassetto := false, true
	visita(td, func(n *html.Node) bool {
		if annidata {
			return false
		}
		switch n.Type {
		case html.TextNode:
			if strings.TrimSpace(n.Data) != "" {
				conTesto = true
				if grassetto == 0 {
					tuttoGrassetto = false
				}
			}
			w.scrivi(n.Data)
			return false
		case html.ElementNode:
		default:
			return false
		}
		a := aspettoDi(n)
		if n != td && (elementiSaltati[n.Data] || a.nascosto) {
			return false
		}
		if n.DataAtom == atom.Table {
			annidata = true
			return false
		}
		if n.DataAtom == atom.A && len(*collegamenti) < 100 {
			if h := attr(n, "href"); h != "" {
				*collegamenti = append(*collegamenti, h[:min(len(h), 4096)])
			}
		}
		c.destra = c.destra || a.destra
		if a.grassetto {
			grassetto++
		}
		aperti = append(aperti, a.grassetto)
		switch {
		case n.DataAtom == atom.Br:
			w.aCapo(true)
		case elementiBlocco[n.DataAtom]:
			w.aCapo(false)
		}
		return true
	}, func(n *html.Node) {
		if aperti[len(aperti)-1] {
			grassetto--
		}
		aperti = aperti[:len(aperti)-1]
		if elementiBlocco[n.DataAtom] {
			w.aCapo(false)
		}
	})
	if annidata {
		return c, true
	}
	w.aCapo(false)
	piene := w.righe[:0]
	for _, r := range w.righe {
		if r != "" {
			piene = append(piene, r)
		}
	}
	c.testo = strings.Join(piene, "\n")
	c.grassetto = conTesto && tuttoGrassetto
	return c, false
}

// togliRigheVuote toglie le righe fatte solo di celle vuote (la riga fantasma di Word, gli
// spaziatori) e riconta i rowspan sulle righe rimaste, limitandoli a quelle che restano.
func togliRigheVuote(righe [][]cellaHTML) [][]cellaHTML {
	n := len(righe)
	prima := make([]int, n+1) // prima[i]: righe tenute fra 0 e i-1
	tenuta := make([]bool, n)
	for i, r := range righe {
		for _, c := range r {
			if c.testo != "" {
				tenuta[i] = true
				break
			}
		}
		prima[i+1] = prima[i]
		if tenuta[i] {
			prima[i+1]++
		}
	}
	var out [][]cellaHTML
	for i, r := range righe {
		if !tenuta[i] {
			continue
		}
		for k := range r {
			fine := n
			if rs := r[k].rowspan; rs > 0 && i+rs < n {
				fine = i + rs
			}
			r[k].rowspan = max(prima[fine]-prima[i], 1)
		}
		out = append(out, r)
	}
	return out
}

// intestazione: la riga 0 è l'intestazione se è fatta tutta di <th>, o se ogni sua cella non vuota
// è in grassetto e almeno una cella non vuota delle righe dopo non lo è (una tabella tutta in
// grassetto non ha intestazione, ha uno stile).
func intestazione(righe [][]cellaHTML) bool {
	r0 := righe[0]
	if len(r0) == 0 {
		return false
	}
	tuttiTh := true
	for _, c := range r0 {
		if !c.th {
			tuttiTh = false
			break
		}
	}
	if tuttiTh {
		return true
	}
	piene := 0
	for _, c := range r0 {
		if c.testo == "" {
			continue
		}
		if !c.grassetto {
			return false
		}
		piene++
	}
	if piene == 0 {
		return false
	}
	for _, r := range righe[1:] {
		for _, c := range r {
			if c.testo != "" && !c.grassetto {
				return true
			}
		}
	}
	return false
}

// vista è la tabella come la vede la pagina: celle troncate a 500 rune, numeri a destra.
func (t *tabellaHTML) vista() *Tabella {
	v := &Tabella{Colonne: t.colonne, Intestazione: t.intestazione, Origine: OrigineHTML}
	for _, r := range t.righe {
		rt := RigaTabella{Celle: make([]Cella, 0, len(r))}
		for _, c := range r {
			testo, troncata := tronca(c.testo, maxRuneCella)
			rt.Celle = append(rt.Celle, Cella{
				Testo:    testo,
				Colspan:  c.colspan,
				Rowspan:  c.rowspan,
				Numero:   c.testo != "" && (c.destra || numerico(c.testo)),
				Troncata: troncata,
			})
		}
		v.Righe = append(v.Righe, rt)
	}
	return v
}

// ---------------------------------------------------------------- il testo dall'HTML

// scrittore accumula testo come lo mostrerebbe un browser: spazi compressi, righe spezzate sui
// confini di blocco. Oltre limite byte smette di scrivere (il chiamante vede il testo lungo e lo
// scarta).
type scrittore struct {
	righe   []string
	riga    strings.Builder
	spazio  bool
	limite  int
	scritti int
}

func (w *scrittore) pieno() bool { return w.limite > 0 && w.scritti >= w.limite }

func (w *scrittore) scrivi(s string) {
	for _, r := range s {
		if unicode.IsSpace(r) {
			w.spazio = w.spazio || w.riga.Len() > 0
			continue
		}
		if w.pieno() {
			return
		}
		if w.spazio {
			w.riga.WriteByte(' ')
			w.scritti++
			w.spazio = false
		}
		n, _ := w.riga.WriteRune(r)
		w.scritti += n
	}
}

// scriviPre scrive il testo di un <pre>: gli spazi e gli a capo sono quelli dell'autore.
func (w *scrittore) scriviPre(s string) {
	for _, r := range s {
		if w.pieno() {
			return
		}
		switch r {
		case '\r':
			continue
		case '\n':
			w.aCapo(true)
			continue
		case '\u00a0', '\u2007', '\u202f':
			r = ' '
		}
		n, _ := w.riga.WriteRune(r)
		w.scritti += n
	}
}

// aCapo chiude la riga corrente; forzato la chiude anche se è vuota (una riga vuota voluta).
func (w *scrittore) aCapo(forzato bool) {
	if w.riga.Len() > 0 || forzato {
		w.righe = append(w.righe, w.riga.String())
		w.scritti++
		w.riga.Reset()
	}
	w.spazio = false
}

func (w *scrittore) linea(s string) {
	w.aCapo(false)
	w.righe = append(w.righe, s)
	w.scritti += len(s) + 1
}

// testo è il corpo HTML come righe di testo, per quando il testo semplice manca: paragrafi come
// righe, le tabelle di dati come righe di celle separate da TAB (così l'ancoraggio le ritrova
// identiche, e dopo tutto va come per un corpo di testo).
func (d *documentoHTML) testo() string {
	if d.radice == nil {
		return testoDaToken(d.sorgente)
	}
	w := &scrittore{limite: LimiteTesto + 1}
	pre := 0
	visita(d.radice, func(n *html.Node) bool {
		switch n.Type {
		case html.DocumentNode:
			return true
		case html.TextNode:
			if pre > 0 {
				w.scriviPre(n.Data)
			} else {
				w.scrivi(n.Data)
			}
			return false
		case html.ElementNode:
		default:
			return false
		}
		if saltato(n) {
			return false
		}
		if t := d.dati[n]; t != nil {
			for _, r := range t.righe {
				w.linea(rigaTab(r))
			}
			return false
		}
		switch n.DataAtom {
		case atom.Br:
			w.aCapo(true)
			return false
		case atom.Hr:
			w.linea(strings.Repeat("_", 32))
			return false
		case atom.Pre:
			pre++
		}
		if elementiBlocco[n.DataAtom] {
			w.aCapo(false)
		}
		return true
	}, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.DataAtom {
		case atom.Pre:
			pre--
			w.aCapo(false)
		case atom.P:
			// Un paragrafo di Word (MsoNormal) è una riga: le righe vuote Word le scrive come
			// paragrafi vuoti. Un <p> di un altro client ha il suo margine: una riga vuota dopo.
			w.aCapo(true)
			if !strings.HasPrefix(strings.ToLower(attr(n, "class")), "mso") {
				w.aCapo(true)
			}
		default:
			if elementiBlocco[n.DataAtom] {
				w.aCapo(false)
			}
		}
	})
	w.aCapo(false)
	return strings.Join(w.righe, "\n")
}

func rigaTab(r []cellaHTML) string {
	celle := make([]string, len(r))
	for i, c := range r {
		celle[i] = strings.ReplaceAll(c.testo, "\n", " ")
	}
	return strings.Join(celle, "\t")
}

// testoDaToken è il ripiego quando il parser rifiuta il documento: il testo dai token, senza
// tabelle e senza sapere cosa è nascosto. Script e stili restano fuori comunque (il loro contenuto
// è un unico token di testo grezzo, subito dopo l'apertura).
func testoDaToken(h string) string {
	z := html.NewTokenizer(strings.NewReader(h))
	w := &scrittore{limite: LimiteTesto + 1}
	grezzo := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			w.aCapo(false)
			return strings.Join(w.righe, "\n")
		case html.TextToken:
			if !grezzo {
				w.scrivi(string(z.Text()))
			}
			grezzo = false
		case html.StartTagToken, html.SelfClosingTagToken:
			nome, _ := z.TagName()
			a := atom.Lookup(nome)
			grezzo = tt == html.StartTagToken && elementiGrezzi[a]
			switch {
			case a == atom.Br:
				w.aCapo(true)
			case a == atom.Hr:
				w.linea(strings.Repeat("_", 32))
			case elementiBlocco[a]:
				w.aCapo(false)
			}
		case html.EndTagToken:
			nome, _ := z.TagName()
			a := atom.Lookup(nome)
			grezzo = false
			if a == atom.P {
				w.aCapo(true)
			} else if elementiBlocco[a] {
				w.aCapo(false)
			}
		default:
			grezzo = false
		}
	}
}
