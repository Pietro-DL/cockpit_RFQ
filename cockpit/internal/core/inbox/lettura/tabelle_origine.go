package lettura

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// LE TABELLE CON LA LORO ORIGINE (giro 5, A1b.3; piano A, 5.4.4)
//
// vista() dà la tabella come la vede la pagina: celle, colspan, rowspan, numeri a destra. Al motore A non basta:
// una cella è un'evidenza, e un'evidenza deve poter dire da dove viene (R28 b: una cella è una sola evidenza,
// con la sua origine precisa). Qui si leggono le stesse tabelle di dati, con gli stessi criteri, e in più si
// conservano gli indici di tabella, riga e cella, il percorso nel DOM e le righe del testo a cui ogni cella si
// aggancia.
//
// Due scelte, perché tabelle_html.go non cambi di un byte (5.1 «No»; 5.10 n.5):
//   - la lettura delle celle è una variante del ciclo di esaminaTabella che tiene i nodi: stessi aiuti (righeDi,
//     leggiCella, saltato, togliRigheVuote, intestazione, èRumoreDiTabella) e stessi limiti. leggiCella porta un
//     totale corrente dei caratteri fra una cella e l'altra, quindi il ciclo si rifà per intero e non si chiama
//     cella per cella. Il rischio di deriva dalla vista lo ferma l'equivalenza con vista() (A1b-10);
//   - l'aggancio è quello di ancora() (ancoraggio.go:79-110), con le stesse parole (parole, flusso, schema,
//     cerca), ma sulle righe del Taglio, cioè sul corpo ORIGINALE, e non sulle stringhe ripulite di TagliaCatena:
//     così le righe di una cella sono righe di corpo_testo, e l'adattatore può darle un intervallo in byte.
//
// Le righe di una cella: ogni cella occupa un tratto noto dello schema di parole, e il flusso dà la riga di ogni
// parola; le righe della cella sono [prima, ultima+1). Una cella vuota non ha righe. Una cella «coincide» quando
// le sue parole sono esattamente quelle delle sue righe (prima parola di una riga, ultima di un'altra): solo
// allora l'adattatore può darle un intervallo esatto (PosTabella.Esatto).
//
// Nessuna tabella si aggancia per vicinanza o per somiglianza: o le sue parole si ritrovano tutte, di seguito,
// nello stesso ordine, da inizio a fine riga, o la tabella resta non agganciata (principio della provenance,
// 5.0). Una tabella che si ritrova solo cercando su tutto il testo, a cavallo del taglio fra parte corrente e
// storia, non si aggancia: si segnala, e la diagnostica la scrive l'adattatore.

// TabellaOrigine: una tabella di dati del corpo HTML, nelle stesse posizioni e con le stesse celle di vista().
type TabellaOrigine struct {
	Celle        []CellaOrigine
	Intestazione bool    // la riga 0 è l'intestazione, come in vista()
	Agganciata   bool    // le parole della tabella si ritrovano nel testo, come in ancora()
	ACavallo     bool    // si ritrova solo a cavallo del taglio: non agganciata, da segnalare [+3, 5.4.4]
	Righe        *[2]int // le righe [da, a) del Taglio su cui la tabella si aggancia; nil se non agganciata
}

// CellaOrigine: una cella con la sua origine. Gli indici sono 0-based (l'adattatore li porta a 1-based nella
// PosTabella della foglia); le righe sono indici in Taglio.Righe.
type CellaOrigine struct {
	Tabella     int     // la tabella fra quelle restituite, in ordine di documento
	TabellaHTML int     // la tabella fra tutti gli elementi <table> del documento, in ordine di documento
	Riga        int     // la riga dopo la pulizia delle righe vuote, come in vista()
	RigaHTML    int     // la <tr> originale fra le righe della tabella (righeDi), prima della pulizia
	Cella       int     // la posizione della cella nella sua riga
	Colonna     int     // la colonna nella griglia della tabella pulita, contando colspan e rowspan sopra
	Colspan     int     // come in vista(): già limitato
	Rowspan     int     // come in vista(): già ricontato sulle righe rimaste
	Testo       string  // il testo intero della cella (vista() lo tronca a 500 rune per la pagina)
	Percorso    string  // nel DOM, per esempio html/body/div[1]/table[1]/tbody[1]/tr[3]/td[2]
	Th          bool    // la cella è un <th>
	Numero      bool    // come in vista(): da allineare a destra
	Righe       *[2]int // le righe [da, a) del Taglio con le parole della cella; nil se vuota o non agganciata
	Coincide    bool    // le parole della cella sono esattamente quelle delle sue righe [+3, 5.4.4]
}

// TabelleConOrigine legge le tabelle di dati come vista() (tabelle_html.go:500-517) e in più conserva gli
// indici di tabella, riga e cella, il percorso nel DOM e le righe del testo originale a cui ogni cella si
// aggancia. Le righe sono quelle del Taglio, sul corpo originale (non le stringhe ripulite di TagliaCatena).
// L'aggancio cerca prima nella parte corrente e poi nella storia, come ancora (ancoraggio.go:79-110).
//
// testo è il testo su cui t è stato calcolato: il corpo_testo, oppure, quando è vuoto, TestoDaHTML(html), come
// fa Presenta. Gli stessi limiti di Presenta: oltre LimiteTesto, o con un HTML vuoto, oltre LimiteHTML o senza
// nessuna «<table», nessuna tabella. Con un Taglio che non è di questo testo, le tabelle ci sono ma nessuna si
// aggancia.
func TabelleConOrigine(testo, html string, t classificazione.Taglio) []TabellaOrigine {
	if len(testo) > LimiteTesto || html == "" || len(html) > LimiteHTML || !contieneTabella(html) {
		return nil
	}
	tabelle := leggiTabelleConOrigine(html)
	if len(tabelle) == 0 {
		return nil
	}
	out := make([]TabellaOrigine, len(tabelle))
	for i, tc := range tabelle {
		out[i] = TabellaOrigine{Celle: tc.celle(i), Intestazione: tc.tab.intestazione}
	}
	agganciaConOrigine(testo, t, tabelle, out)
	return out
}

// TestoDaHTML: il testo che Presenta ricava dall'HTML quando il testo semplice manca, con le stesse regole e
// gli stessi limiti (lettura.go:157-184; tabelle_html.go:592-659): le tabelle di dati come righe di celle
// separate da TAB, il ripiego sui token se il parser rifiuta il documento. "" se l'HTML è vuoto, supera
// LimiteHTML, non dà testo, o dà un testo oltre LimiteTesto (dove Presenta mostrerebbe solo l'originale). Serve
// all'adattatore della mail per i corpi vuoti con solo HTML [+3, 5.4.4].
func TestoDaHTML(html string) string {
	if html == "" || len(html) > LimiteHTML {
		return ""
	}
	t := leggiHTML(html).testo()
	if strings.TrimSpace(t) == "" || len(t) > LimiteTesto {
		return ""
	}
	return t
}

// tabellaConOrigine: una tabella di dati come la dà esaminaTabella, più i nodi e gli indici che la vista perde.
type tabellaConOrigine struct {
	tab         *tabellaHTML
	nodo        *html.Node     // l'elemento <table>
	ordinale    int            // fra tutti gli elementi <table> del documento
	righeHTML   []int          // per ogni riga rimasta, la sua <tr> fra quelle di righeDi
	nodiCelle   [][]*html.Node // per ogni riga rimasta, i <td>/<th> delle sue celle
	colonneCell [][]int        // per ogni cella, la colonna nella griglia
}

// leggiTabelleConOrigine è leggiHTML (tabelle_html.go:95-123) con la variante di esaminaTabella: le stesse
// tabelle di dati, nello stesso ordine, con lo stesso tetto di maxTabelle. Se il parser rifiuta il documento,
// nessuna tabella, come in leggiHTML.
func leggiTabelleConOrigine(h string) []*tabellaConOrigine {
	radice, err := html.Parse(strings.NewReader(h))
	if err != nil {
		return nil
	}
	// L'ordinale di ogni <table> fra tutte quelle del documento, nascoste e annidate comprese.
	ordinali := map[*html.Node]int{}
	visita(radice, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.DataAtom == atom.Table {
			ordinali[n] = len(ordinali)
		}
		return true
	}, nil)
	var out []*tabellaConOrigine
	visita(radice, func(n *html.Node) bool {
		switch n.Type {
		case html.DocumentNode:
			return true
		case html.ElementNode:
		default:
			return false
		}
		if saltato(n) {
			return false
		}
		if n.DataAtom == atom.Table && len(out) < maxTabelle {
			if tc := esaminaTabellaConOrigine(n); tc != nil {
				tc.ordinale = ordinali[n]
				out = append(out, tc)
				return false
			}
		}
		return true
	}, nil)
	return out
}

// esaminaTabellaConOrigine è il ciclo di esaminaTabella (tabelle_html.go:249-321), rifatto per intero, che
// tiene per ogni cella il suo nodo e per ogni riga la sua <tr> prima della pulizia. Criteri e limiti sono gli
// stessi, nello stesso ordine; nil dove esaminaTabella dà nil.
func esaminaTabellaConOrigine(t *html.Node) *tabellaConOrigine {
	var righe [][]cellaHTML
	var nodi [][]*html.Node
	var collegamenti []string
	totale, piene := 0, 0
	for _, tr := range righeDi(t) {
		var riga []cellaHTML
		var nodiRiga []*html.Node
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
			nodiRiga = append(nodiRiga, c)
		}
		if !vuota {
			if piene++; piene > maxRigheTabella {
				return nil
			}
		}
		righe = append(righe, riga)
		nodi = append(nodi, nodiRiga)
	}
	// Gli indici si registrano prima di togliRigheVuote, che rinumera le righe: una riga resta se ha almeno una
	// cella non vuota, la stessa regola di togliRigheVuote (tabelle_html.go:425-456).
	var righeHTML []int
	var nodiTenuti [][]*html.Node
	for i, r := range righe {
		for _, c := range r {
			if c.testo != "" {
				righeHTML = append(righeHTML, i)
				nodiTenuti = append(nodiTenuti, nodi[i])
				break
			}
		}
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
	return &tabellaConOrigine{
		tab:         &tabellaHTML{righe: righe, colonne: colonne, intestazione: intestazione(righe)},
		nodo:        t,
		righeHTML:   righeHTML,
		nodiCelle:   nodiTenuti,
		colonneCell: colonneDellaGriglia(righe),
	}
}

// colonneDellaGriglia: la colonna di ogni cella nella griglia della tabella pulita, come la conta il modello delle
// tabelle HTML: una cella va nella prima colonna libera della sua riga, dopo quelle occupate dai rowspan delle
// righe sopra, e occupa colspan colonne per rowspan righe. Le colonne di <col> e <colgroup> non contano.
func colonneDellaGriglia(righe [][]cellaHTML) [][]int {
	occupate := make([][]bool, len(righe))
	out := make([][]int, len(righe))
	for r, riga := range righe {
		out[r] = make([]int, len(riga))
		col := 0
		for k, c := range riga {
			for col < len(occupate[r]) && occupate[r][col] {
				col++
			}
			out[r][k] = col
			for dr := 0; dr < c.rowspan && r+dr < len(righe); dr++ {
				for dc := 0; dc < c.colspan; dc++ {
					occupa := &occupate[r+dr]
					for len(*occupa) <= col+dc {
						*occupa = append(*occupa, false)
					}
					(*occupa)[col+dc] = true
				}
			}
			col += c.colspan
		}
	}
	return out
}

// celle: le celle della tabella i, in ordine di riga e di cella (l'ordine di vista() e di schema()).
func (tc *tabellaConOrigine) celle(i int) []CellaOrigine {
	var out []CellaOrigine
	for r, riga := range tc.tab.righe {
		for k, c := range riga {
			out = append(out, CellaOrigine{
				Tabella:     i,
				TabellaHTML: tc.ordinale,
				Riga:        r,
				RigaHTML:    tc.righeHTML[r],
				Cella:       k,
				Colonna:     tc.colonneCell[r][k],
				Colspan:     c.colspan,
				Rowspan:     c.rowspan,
				Testo:       c.testo,
				Percorso:    percorsoNelDOM(tc.nodiCelle[r][k]),
				Th:          c.th,
				Numero:      c.testo != "" && (c.destra || numerico(c.testo)),
			})
		}
	}
	return out
}

// percorsoNelDOM: l'indirizzo dell'elemento n dal documento, con l'indice 1-based fra i fratelli dello stesso tag
// (html e body senza indice: il parser ne mette sempre uno solo). Dipende dalla versione del parser HTML, che per
// questo entra nella versione degli adattatori (5.4.4).
func percorsoNelDOM(n *html.Node) string {
	var pezzi []string
	for e := n; e != nil && e.Type == html.ElementNode; e = e.Parent {
		if e.DataAtom == atom.Html || e.DataAtom == atom.Body {
			pezzi = append(pezzi, e.Data)
			continue
		}
		k := 1
		for f := e.PrevSibling; f != nil; f = f.PrevSibling {
			if f.Type == html.ElementNode && f.Data == e.Data && f.Namespace == e.Namespace {
				k++
			}
		}
		var b strings.Builder
		b.WriteString(e.Data)
		b.WriteByte('[')
		scriviIntero(&b, k)
		b.WriteByte(']')
		pezzi = append(pezzi, b.String())
	}
	for i, j := 0, len(pezzi)-1; i < j; i, j = i+1, j-1 {
		pezzi[i], pezzi[j] = pezzi[j], pezzi[i]
	}
	return strings.Join(pezzi, "/")
}

// scriviIntero scrive n ≥ 0 in decimale: il package non importa strconv, e il file nuovo non porta import nuovi.
func scriviIntero(b *strings.Builder, n int) {
	if n >= 10 {
		scriviIntero(b, n/10)
	}
	b.WriteByte(byte('0' + n%10))
}

// agganciaConOrigine è ancora() (ancoraggio.go:79-110) sulle righe del Taglio: ogni tabella, in ordine di
// documento, si cerca dopo quella trovata prima, nella parte corrente e poi nella storia; una tabella che non si
// trova non sposta niente. Le sezioni sono le righe [0, RigaTaglio) e [RigaTaglio, fine) del corpo originale
// (tutto il corpo nella parte corrente se non c'è storia): le parole sono quelle delle stringhe di
// TagliaCatena, perché parole() normalizza come normalizza e TrimSpace non toglie parole.
func agganciaConOrigine(testo string, t classificazione.Taglio, tabelle []*tabellaConOrigine, out []TabellaOrigine) {
	n := len(t.Righe)
	if n == 0 || t.Righe[0].Inizio != 0 || t.Righe[n-1].FineTerminatore != len(testo) {
		return // un Taglio di un altro testo: nessun aggancio
	}
	grezze := make([]string, n)
	for k, r := range t.Righe {
		if r.Inizio < 0 || r.Fine < r.Inizio || r.Fine > len(testo) {
			return
		}
		grezze[k] = testo[r.Inizio:r.Fine]
	}
	taglio := n
	if t.RigaTaglio >= 0 && t.RigaTaglio <= n {
		taglio = t.RigaTaglio
	}
	basi := [2]int{0, taglio}
	voc := map[string]int32{}
	sezioni := [2]*sezione{{grezze: grezze[:taglio]}, {grezze: grezze[taglio:]}}
	flussi := [2]*flusso{sezioni[0].flusso(voc), sezioni[1].flusso(voc)}
	tutto := unisciFlussi(flussi[0], flussi[1], taglio)
	occupate := make([]bool, n)
	sez, pos := 0, 0
	for i, tc := range tabelle {
		schema, ok := tc.tab.schema(voc)
		if !ok {
			continue
		}
		trovata := false
		for k := sez; k < 2; k++ {
			da := 0
			if k == sez {
				da = pos
			}
			f := flussi[k]
			inizio := cerca(f, da, schema)
			if inizio < 0 {
				continue
			}
			trovata = true
			fine := inizio + len(schema)
			rda, ra := basi[k]+int(f.riga[inizio]), basi[k]+int(f.riga[fine-1])+1
			if righeLibere(occupate, rda, ra) {
				for r := rda; r < ra; r++ {
					occupate[r] = true
				}
				sez, pos = k, fine
				out[i].Agganciata = true
				out[i].Righe = &[2]int{rda, ra}
				righeDelleCelle(tc, f, inizio, basi[k], out[i].Celle)
			}
			break
		}
		if trovata {
			continue
		}
		// Nelle due sezioni no: cercata su tutto il testo, dalla stessa posizione, una tabella che c'è sta per
		// forza a cavallo del taglio. Non si aggancia e non sposta niente; si segnala.
		g := pos
		if sez == 1 {
			g += len(flussi[0].id)
		}
		if inizio := cerca(tutto, g, schema); inizio >= 0 && inizio < len(flussi[0].id) && inizio+len(schema) > len(flussi[0].id) {
			out[i].ACavallo = true
		}
	}
}

// unisciFlussi: il flusso di tutto il testo, con le righe della storia spostate dopo quelle della parte corrente.
func unisciFlussi(a, b *flusso, base int) *flusso {
	f := &flusso{
		id:     append(append([]int32(nil), a.id...), b.id...),
		riga:   append([]int32(nil), a.riga...),
		apre:   append(append([]bool(nil), a.apre...), b.apre...),
		chiude: append(append([]bool(nil), a.chiude...), b.chiude...),
	}
	for _, r := range b.riga {
		f.riga = append(f.riga, r+int32(base))
	}
	return f
}

func righeLibere(occupate []bool, da, a int) bool {
	if da < 0 || a > len(occupate) || da >= a {
		return false
	}
	for r := da; r < a; r++ {
		if occupate[r] {
			return false
		}
	}
	return true
}

// righeDelleCelle: le righe di ogni cella della tabella agganciata a partire dalla parola inizio del flusso f,
// nella sezione che comincia alla riga base. Le parole delle celle sono lo schema, nello stesso ordine.
func righeDelleCelle(tc *tabellaConOrigine, f *flusso, inizio, base int, celle []CellaOrigine) {
	k := inizio
	for i := range celle {
		w := len(parole(celle[i].Testo))
		if w == 0 {
			continue
		}
		prima, ultima := k, k+w-1
		celle[i].Righe = &[2]int{base + int(f.riga[prima]), base + int(f.riga[ultima]) + 1}
		celle[i].Coincide = f.apre[prima] && f.chiude[ultima]
		k += w
	}
}
