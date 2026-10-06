package motorea

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"unicode/utf8"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// pianoForma: la regex di una coppia (famiglia, forma) e la mappa dei suoi gruppi. È la rappresentazione
// privata e minima della forma, non una copia di FormaCodice (par.12): ogni gruppo porta solo l'operazione da
// fare sul testo catturato.
//   - Ogni parte, ogni segmento della base e ogni pezzo interno che si legge ha un gruppo generato dal
//     compilatore («g0», «g1»…): nomi unici per costruzione, perché i pattern dei clienti non hanno gruppi
//     (T12). g0 è l'intervallo della lettura.
//   - La regex è «\A(?:<parti>)(?:<confine destro>|\z)», compilata con regexp.Compile (mai MustCompile, T18)
//     e con Longest(): a parità d'inizio vince la lettura più lunga che rispetta il confine, qualunque sia
//     l'ordine dei letterali e delle parti facoltative (T6). reIntera è la stessa sequenza ancorata anche
//     alla fine: serve ad allungare la lettura quando il confine destro ha consumato un carattere che poteva
//     essere suo (scansione.go).
//   - Le alternative letterali sono scritte con QuoteMeta, dalla più lunga alla più corta (T16).
//   - Un'etichetta diventa il letterale più fino a SpaziMax spazi U+0020 («[ ]{0,N}»); nessun \s (T10).
type pianoForma struct {
	famiglia, namespace, forma string
	categorie                  []grammatica.Categoria
	completa                   bool
	mancanti                   []string
	maiuscolo                  bool // la base si normalizza con le maiuscole ASCII
	confinePrima, confineDopo  grammatica.ClasseConfine
	re, reIntera               *regexp.Regexp
	prime                      insiemeRune // le rune con cui una lettura può cominciare
	maxByte                    int         // i byte di una lettura al più; -1 se la sequenza non ha un limite
	gruppi                     []gruppo
	scrittura                  []parteScritta // le parti come le legge il compositore (componi.go, A1c B3; R63 B)
}

// operazione: che cosa fare del testo di un gruppo. Sono poche, e nessuna nomina un cliente (par.12): il
// tipo di una decorazione passa nella lettura come etichetta, la ripetizione si confronta con la base, il
// token si conserva, l'affisso si attribuisce secondo il selettore.
type operazione int

const (
	opLettura           operazione = iota // l'intervallo della lettura (g0)
	opBase                                // la base: originale e normalizzata
	opSegmento                            // un segmento della base o della ripetizione (padre)
	opRipetizione                         // la base scritta di nuovo: da confrontare con la base (D2)
	opEtichetta                           // l'etichetta con i suoi spazi
	opValoreEtichetta                     // il letterale dell'etichetta
	opAffisso                             // da attribuire secondo il selettore (parte 1 §5.3)
	opMarcatore                           // da conservare (D1, R7)
	opToken                               // da conservare, mai attribuire (Q1, D10)
	opDecorazione                         // da conservare con il suo tipo, fuori dall'identità
	opRevisione                           // la revisione, con il separatore
	opValoreRevisione                     // i segmenti letti: la stringa normalizzata
	opSegmentoRevisione                   // un segmento nominato della revisione (forte, debole)
	opTokenRevisione                      // un token sospeso: da conservare, da verificare (D5)
)

// gruppo: un gruppo della regex con la sua operazione. padre è l'indice del gruppo che lo contiene, per i
// pezzi che ne hanno uno (segmenti, valore dell'etichetta, pezzi della revisione), altrimenti -1.
type gruppo struct {
	numero       int // il numero del gruppo nella regex (1 per g0)
	op           operazione
	padre        int
	nome         string                          // il nome del segmento
	regola       string                          // l'ID della regola (etichetta, affisso, decorazione, revisione)
	tipo         string                          // il tipo della decorazione, come etichetta
	riserva      string                          // la riserva della decorazione
	posizione    string                          // dell'affisso
	attribuzione []string                        // i selettori dove l'affisso si attribuisce
	valore       *grammatica.ValoreQualificatore // dell'affisso
	rev          *regolaRevisione
}

// regolaRevisione: una regola di revisione abbassata, condivisa dalle forme della famiglia che la usano.
type regolaRevisione struct {
	id, sorgente string
	separatori   []string
	segmenti     []grammatica.SegmentoRevisione
	token        []string       // i pattern dei token sospesi
	reToken      *regexp.Regexp // i token sospesi su tutto il testo dopo il separatore; nil se non ce ne sono
	equivalenze  [][2]string    // solo dichiarate; nil se nessuna
}

// pianoRevisione: una regola di revisione in campo separato su un selettore (D-07). Legge tutto il testo del
// campo. In A1a serve alla verifica degli esempi; in A1b la stessa funzione darà l'attributo.
type pianoRevisione struct {
	famiglia string
	rev      *regolaRevisione
	re       *regexp.Regexp
	gruppi   []gruppo
}

// costruttore: scrive la regex di un piano e ne registra i gruppi, nell'ordine in cui si aprono, così il
// numero di ogni gruppo è noto per costruzione.
type costruttore struct {
	sb     strings.Builder
	gruppi []gruppo
	piega  bool // maiuscole indifferenti_ascii della base
}

func (c *costruttore) apri(g gruppo) int {
	i := len(c.gruppi)
	g.numero = i + 1
	c.gruppi = append(c.gruppi, g)
	fmt.Fprintf(&c.sb, "(?P<g%d>", i)
	return i
}

func (c *costruttore) chiudi()         { c.sb.WriteByte(')') }
func (c *costruttore) scrivi(s string) { c.sb.WriteString(s) }

// alternativa: i letterali come alternative, dalla più lunga alla più corta, con QuoteMeta (T16).
func alternativa(letterali []string) string {
	l := append([]string(nil), letterali...)
	sort.SliceStable(l, func(i, j int) bool {
		if len(l[i]) != len(l[j]) {
			return len(l[i]) > len(l[j])
		}
		return l[i] < l[j]
	})
	for i := range l {
		l[i] = regexp.QuoteMeta(l[i])
	}
	return "(?:" + strings.Join(l, "|") + ")"
}

// letteraleBase: un letterale della base (segmento o separatore), con le maiuscole della base.
func (c *costruttore) letteraleBase(s string) string {
	if !c.piega {
		return regexp.QuoteMeta(s)
	}
	var b strings.Builder
	for _, r := range s {
		if altra, ok := altraMaiuscola(r); ok {
			fmt.Fprintf(&b, "[%c%c]", min(r, altra), max(r, altra))
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// patternBase: un pattern della base, con le maiuscole della base. Le maiuscole indifferenti si ottengono
// aggiungendo alle lettere ASCII l'altra maiuscola, mai con (?i), che piega anche lettere fuori dall'ASCII
// (T9).
func (c *costruttore) patternBase(p string) (string, error) {
	if !c.piega {
		return "(?:" + p + ")", nil
	}
	albero, err := syntax.Parse(p, syntax.Perl)
	if err != nil {
		return "", err
	}
	return "(?:" + piegaASCII(albero).String() + ")", nil
}

// base scrive la base (o la sua ripetizione) con i segmenti dati: tutti, o la proiezione di una forma
// parziale.
func (c *costruttore) base(op operazione, segmenti []grammatica.SegmentoBase) error {
	i := c.apri(gruppo{op: op, padre: -1})
	for _, s := range segmenti {
		if s.Separatore != "" {
			c.scrivi(c.letteraleBase(s.Separatore))
		}
		c.apri(gruppo{op: opSegmento, padre: i, nome: s.Nome})
		if s.Letterale != "" {
			c.scrivi(c.letteraleBase(s.Letterale))
		} else {
			p, err := c.patternBase(s.Pattern)
			if err != nil {
				return err
			}
			c.scrivi(p)
		}
		c.chiudi()
	}
	c.chiudi()
	return nil
}

// revisione scrive una revisione: un separatore fra quelli dichiarati, se ce ne sono, poi i segmenti in fila
// oppure un token sospeso.
func (c *costruttore) revisione(r *regolaRevisione) {
	i := c.apri(gruppo{op: opRevisione, padre: -1, regola: r.id, rev: r})
	if len(r.separatori) > 0 {
		c.scrivi(alternativa(r.separatori))
	}
	c.scrivi("(?:")
	if len(r.segmenti) > 0 {
		v := c.apri(gruppo{op: opValoreRevisione, padre: i})
		for _, s := range r.segmenti {
			c.apri(gruppo{op: opSegmentoRevisione, padre: v, nome: s.Nome})
			c.scrivi("(?:" + s.Pattern + ")")
			c.chiudi()
		}
		c.chiudi()
		if len(r.token) > 0 {
			c.scrivi("|")
		}
	}
	if len(r.token) > 0 {
		c.apri(gruppo{op: opTokenRevisione, padre: i})
		c.scrivi(alternativaPattern(r.token))
		c.chiudi()
	}
	c.scrivi(")")
	c.chiudi()
}

// alternativaPattern: pattern già verificati come alternative, ognuno nel suo gruppo non catturante.
func alternativaPattern(pattern []string) string {
	l := make([]string, len(pattern))
	for i, p := range pattern {
		l[i] = "(?:" + p + ")"
	}
	return "(?:" + strings.Join(l, "|") + ")"
}

// abbassaRevisione: la regola di revisione nella forma che il motore usa.
func abbassaRevisione(r grammatica.RegolaRevisione) (*regolaRevisione, error) {
	rr := &regolaRevisione{
		id:         r.ID,
		sorgente:   r.Sorgente,
		separatori: r.Separatori,
		segmenti:   r.Segmenti,
	}
	for _, t := range r.TokenSospesi {
		rr.token = append(rr.token, t.Pattern)
	}
	if len(rr.token) > 0 {
		re, err := regexp.Compile(`\A` + alternativaPattern(rr.token) + `\z`)
		if err != nil {
			return nil, err
		}
		rr.reToken = re
	}
	if len(r.Equivalenze) > 0 {
		rr.equivalenze = append([][2]string(nil), r.Equivalenze...)
	}
	return rr, nil
}

// regoleFamiglia: le regole di una famiglia che le parti citano per ID. Le revisioni sono già abbassate.
type regoleFamiglia struct {
	f         grammatica.FamigliaCodice
	revisioni map[string]*regolaRevisione // per ID: solo per cercare
}

func (r regoleFamiglia) etichetta(id string) *grammatica.Etichetta {
	for i := range r.f.Etichette {
		if r.f.Etichette[i].ID == id {
			return &r.f.Etichette[i]
		}
	}
	return nil
}

func (r regoleFamiglia) affisso(id string) *grammatica.Affisso {
	for i := range r.f.Affissi {
		if r.f.Affissi[i].ID == id {
			return &r.f.Affissi[i]
		}
	}
	return nil
}

func (r regoleFamiglia) decorazione(id string) *grammatica.Decorazione {
	for i := range r.f.Decorazioni {
		if r.f.Decorazioni[i].ID == id {
			return &r.f.Decorazioni[i]
		}
	}
	return nil
}

// segmentiDellaForma: i segmenti della base che la forma legge, cioè tutti meno i mancanti in coda.
func segmentiDellaForma(b grammatica.Base, fo grammatica.FormaCodice) []grammatica.SegmentoBase {
	var out []grammatica.SegmentoBase
	for _, s := range b.Segmenti {
		if !contiene(fo.SegmentiMancanti, s.Nome) {
			out = append(out, s)
		}
	}
	return out
}

// compilaForma abbassa una forma attiva nel suo piano. Le parti sono già validate (Valida): qui un rif che
// non si risolve è un errore interno, e la forma non entra nel motore.
func compilaForma(r regoleFamiglia, fo grammatica.FormaCodice) (*pianoForma, error) {
	f := r.f
	p := &pianoForma{
		famiglia:     f.ID,
		namespace:    f.Namespace,
		forma:        fo.ID,
		categorie:    f.Categorie,
		completa:     fo.Completa,
		mancanti:     fo.SegmentiMancanti,
		maiuscolo:    f.Base.Normalizza == grammatica.NormalizzaMaiuscolo,
		confinePrima: f.Base.ConfinePrima,
		confineDopo:  f.Base.ConfineDopo,
	}
	if fo.ConfinePrima != nil {
		p.confinePrima = *fo.ConfinePrima
	}
	if fo.ConfineDopo != nil {
		p.confineDopo = *fo.ConfineDopo
	}
	destro, ok := frammentoDestro(p.confineDopo)
	if !ok || !classeNota(p.confinePrima) {
		return nil, fmt.Errorf("classe di confine %q o %q sconosciuta al motore", p.confinePrima, p.confineDopo)
	}
	segmenti := segmentiDellaForma(f.Base, fo)

	c := &costruttore{piega: f.Base.Maiuscole == grammatica.MaiuscoleIndifferentiASCII}
	c.apri(gruppo{op: opLettura, padre: -1})
	if err := c.parti(r, fo.Parti, segmenti); err != nil {
		return nil, err
	}
	c.chiudi()
	sequenza := c.sb.String()

	var err error
	if p.re, err = regexp.Compile(`\A` + sequenza + destro); err != nil {
		return nil, err
	}
	if p.reIntera, err = regexp.Compile(`\A` + sequenza + `\z`); err != nil {
		return nil, err
	}
	p.re.Longest()
	p.reIntera.Longest()
	if p.re.NumSubexp() != len(c.gruppi) || p.reIntera.NumSubexp() != len(c.gruppi) {
		return nil, fmt.Errorf("gruppi della regex %d, attesi %d: un pattern contiene un gruppo", p.re.NumSubexp(), len(c.gruppi))
	}
	albero, err := syntax.Parse(sequenza, syntax.Perl)
	if err != nil {
		return nil, err
	}
	if s, vuota := primeRune(albero); !vuota {
		p.prime = s
	} else {
		p.prime = insiemeRune{tutte: true}
	}
	p.maxByte = lunghezzaMassima(albero)
	p.gruppi = c.gruppi
	p.scrittura = scritturaDi(r, fo)
	return p, nil
}

// parti scrive le parti di una forma, o la sequenza interna di un suffisso_documento. Una parte facoltativa
// (Min 0) sta in un gruppo non catturante con «?»: Max è sempre 1 (Valida).
func (c *costruttore) parti(r regoleFamiglia, parti []grammatica.Parte, segmenti []grammatica.SegmentoBase) error {
	for _, pt := range parti {
		facoltativa := pt.Min == 0
		if facoltativa {
			c.scrivi("(?:")
		}
		switch pt.Tipo {
		case grammatica.TipoParteEtichetta:
			e := r.etichetta(pt.Rif)
			if e == nil {
				return fmt.Errorf("etichetta %q non trovata", pt.Rif)
			}
			i := c.apri(gruppo{op: opEtichetta, padre: -1, regola: e.ID})
			c.apri(gruppo{op: opValoreEtichetta, padre: i})
			c.scrivi(alternativa(e.Letterali))
			c.chiudi()
			if e.SpaziMax > 0 {
				c.scrivi(fmt.Sprintf("[ ]{0,%d}", e.SpaziMax))
			}
			c.chiudi()
		case grammatica.TipoParteAffisso:
			a := r.affisso(pt.Rif)
			if a == nil {
				return fmt.Errorf("affisso %q non trovato", pt.Rif)
			}
			c.apri(gruppo{op: opAffisso, padre: -1, regola: a.ID, posizione: a.Posizione, attribuzione: a.Attribuzione, valore: a.Valore})
			c.scrivi(alternativa(a.Letterali))
			c.chiudi()
		case grammatica.TipoParteBase:
			if err := c.base(opBase, segmenti); err != nil {
				return err
			}
		case grammatica.TipoParteRipetizioneBase:
			if err := c.base(opRipetizione, segmenti); err != nil {
				return err
			}
		case grammatica.TipoParteRevisione:
			rv := r.revisioni[pt.Rif]
			if rv == nil {
				return fmt.Errorf("revisione %q non trovata", pt.Rif)
			}
			c.revisione(rv)
		case grammatica.TipoParteDecorazione:
			d := r.decorazione(pt.Rif)
			if d == nil {
				return fmt.Errorf("decorazione %q non trovata", pt.Rif)
			}
			c.apri(gruppo{op: opDecorazione, padre: -1, regola: d.ID, tipo: d.Tipo, riserva: d.Riserva})
			// Il motore guarda le parti, i pattern e i letterali, mai il tipo, che passa nella lettura come
			// etichetta (par.12). Le parti le ha solo il suffisso_documento, e letterali e pattern sono
			// alternativi: lo garantisce Valida.
			switch {
			case len(d.Parti) > 0:
				if err := c.parti(r, d.Parti, segmenti); err != nil {
					return err
				}
			case d.Pattern != "":
				c.scrivi("(?:" + d.Pattern + ")")
			default:
				c.scrivi(alternativa(d.Letterali))
			}
			c.chiudi()
		case grammatica.TipoParteSeparatore:
			c.scrivi(alternativa(pt.Letterali))
		case grammatica.TipoParteMarcatore:
			c.apri(gruppo{op: opMarcatore, padre: -1})
			c.scrivi(alternativa(pt.Letterali))
			c.chiudi()
		case grammatica.TipoParteToken:
			c.apri(gruppo{op: opToken, padre: -1})
			c.scrivi(alternativa(pt.Letterali))
			c.chiudi()
		default:
			return fmt.Errorf("tipo di parte %q sconosciuto al motore", pt.Tipo)
		}
		if facoltativa {
			c.scrivi(")?")
		}
	}
	return nil
}

// compilaRevisioneCampo abbassa una revisione in campo separato: legge tutto il testo del campo (D-07).
func compilaRevisioneCampo(famiglia string, r *regolaRevisione) (*pianoRevisione, error) {
	c := &costruttore{}
	c.revisione(r)
	re, err := regexp.Compile(`\A` + c.sb.String() + `\z`)
	if err != nil {
		return nil, err
	}
	re.Longest()
	if re.NumSubexp() != len(c.gruppi) {
		return nil, fmt.Errorf("gruppi della regex %d, attesi %d: un pattern contiene un gruppo", re.NumSubexp(), len(c.gruppi))
	}
	return &pianoRevisione{famiglia: famiglia, rev: r, re: re, gruppi: c.gruppi}, nil
}

// leggi applica la revisione in campo separato a tutto il testo del campo; nil se non lo legge.
func (p *pianoRevisione) leggi(testo string) *RevisioneLetta {
	loc := p.re.FindStringSubmatchIndex(testo)
	if loc == nil {
		return nil
	}
	m := match{testo: testo, loc: loc}
	a, b, _ := m.pos(p.gruppi[0])
	return revisioneLetta(p.gruppi, m, 0, p.rev, a, b)
}

// ---- maiuscole indifferenti ASCII (T9) ----

// altraMaiuscola: per una lettera ASCII, la stessa lettera con l'altra maiuscola.
func altraMaiuscola(r rune) (rune, bool) {
	switch {
	case 'a' <= r && r <= 'z':
		return r - ('a' - 'A'), true
	case 'A' <= r && r <= 'Z':
		return r + ('a' - 'A'), true
	}
	return 0, false
}

// piegaASCII: lo stesso albero, con le lettere ASCII dei letterali e delle classi in tutte e due le
// maiuscole. Le rune fuori dall'ASCII restano come sono: la K del Kelvin non diventa una k (T9).
func piegaASCII(re *syntax.Regexp) *syntax.Regexp {
	switch re.Op {
	case syntax.OpLiteral:
		subs := make([]*syntax.Regexp, 0, len(re.Rune))
		for _, r := range re.Rune {
			if altra, ok := altraMaiuscola(r); ok {
				subs = append(subs, &syntax.Regexp{Op: syntax.OpCharClass, Rune: []rune{min(r, altra), min(r, altra), max(r, altra), max(r, altra)}})
			} else {
				subs = append(subs, &syntax.Regexp{Op: syntax.OpLiteral, Rune: []rune{r}})
			}
		}
		if len(subs) == 1 {
			return subs[0]
		}
		return &syntax.Regexp{Op: syntax.OpConcat, Sub: subs}
	case syntax.OpCharClass:
		return &syntax.Regexp{Op: syntax.OpCharClass, Rune: piegaClasse(re.Rune)}
	}
	n := *re
	if len(re.Sub) > 0 {
		n.Sub = make([]*syntax.Regexp, len(re.Sub))
		for i, s := range re.Sub {
			n.Sub[i] = piegaASCII(s)
		}
	}
	return &n
}

// piegaClasse: gli intervalli di una classe, più quelli delle lettere ASCII con l'altra maiuscola, ordinati e
// fusi.
func piegaClasse(intervalli []rune) []rune {
	tutti := append([]rune(nil), intervalli...)
	for i := 0; i+1 < len(intervalli); i += 2 {
		lo, hi := intervalli[i], intervalli[i+1]
		if a, b := max(lo, 'a'), min(hi, 'z'); a <= b {
			tutti = append(tutti, a-('a'-'A'), b-('a'-'A'))
		}
		if a, b := max(lo, 'A'), min(hi, 'Z'); a <= b {
			tutti = append(tutti, a+('a'-'A'), b+('a'-'A'))
		}
	}
	coppie := make([][2]rune, 0, len(tutti)/2)
	for i := 0; i+1 < len(tutti); i += 2 {
		coppie = append(coppie, [2]rune{tutti[i], tutti[i+1]})
	}
	sort.Slice(coppie, func(i, j int) bool { return coppie[i][0] < coppie[j][0] })
	var out []rune
	for _, c := range coppie {
		if n := len(out); n > 0 && c[0] <= out[n-1]+1 {
			out[n-1] = max(out[n-1], c[1])
			continue
		}
		out = append(out, c[0], c[1])
	}
	return out
}

// ---- la lunghezza massima di una lettura ----

// lunghezzaMassima: i byte più lunghi che un testo riconosciuto dall'albero può avere; -1 se non c'è un
// limite. Serve solo a non dare alla regex più testo di quanto una lettura ne possa usare (scansione.go): nel
// dubbio dice -1, e allora la ricerca vede tutto il testo. Ogni runa conta per i byte della più lunga che la
// classe ammette; un byte non valido conta 1 nel testo, quindi meno.
func lunghezzaMassima(re *syntax.Regexp) int {
	const tetto = 1 << 30
	switch re.Op {
	case syntax.OpNoMatch, syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return 0
	case syntax.OpLiteral:
		n := 0
		for _, r := range re.Rune {
			if w := utf8.RuneLen(r); w > 0 && re.Flags&syntax.FoldCase == 0 {
				n += w
			} else {
				n += utf8.UTFMax
			}
		}
		return n
	case syntax.OpCharClass:
		if len(re.Rune) == 0 {
			return 0
		}
		if w := utf8.RuneLen(re.Rune[len(re.Rune)-1]); w > 0 {
			return w // gli intervalli sono ordinati: l'ultimo estremo è la runa più lunga
		}
		return utf8.UTFMax
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return utf8.UTFMax
	case syntax.OpCapture, syntax.OpQuest:
		return lunghezzaMassima(re.Sub[0])
	case syntax.OpRepeat:
		s := lunghezzaMassima(re.Sub[0])
		if re.Max < 0 || s < 0 || (s > 0 && re.Max > tetto/s) {
			return -1
		}
		return re.Max * s
	case syntax.OpConcat, syntax.OpAlternate:
		n := 0
		for _, sub := range re.Sub {
			s := lunghezzaMassima(sub)
			if s < 0 {
				return -1
			}
			if re.Op == syntax.OpConcat {
				n += s
			} else {
				n = max(n, s)
			}
			if n > tetto {
				return -1
			}
		}
		return n
	}
	return -1 // OpStar, OpPlus e ciò che non si conosce: nessun limite
}

// ---- le rune con cui una lettura può cominciare ----

// insiemeRune: un insieme di rune a intervalli [lo, hi]; tutte vuol dire nessun filtro.
type insiemeRune struct {
	tutte      bool
	intervalli []rune
}

func (s insiemeRune) contiene(r rune) bool {
	if s.tutte {
		return true
	}
	for i := 0; i+1 < len(s.intervalli); i += 2 {
		if s.intervalli[i] <= r && r <= s.intervalli[i+1] {
			return true
		}
	}
	return false
}

func (s *insiemeRune) unisci(o insiemeRune) {
	s.tutte = s.tutte || o.tutte
	s.intervalli = append(s.intervalli, o.intervalli...)
}

// primeRune: le rune con cui un testo riconosciuto dall'albero può cominciare, e se l'albero riconosce anche
// la stringa vuota. Serve solo a non provare la regex dove non può cominciare: nel dubbio dà tutte le rune,
// così non toglie mai un inizio buono.
func primeRune(re *syntax.Regexp) (insiemeRune, bool) {
	switch re.Op {
	case syntax.OpNoMatch:
		return insiemeRune{}, false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return insiemeRune{}, true
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return insiemeRune{}, true
		}
		if re.Flags&syntax.FoldCase != 0 {
			return insiemeRune{tutte: true}, false
		}
		return insiemeRune{intervalli: []rune{re.Rune[0], re.Rune[0]}}, false
	case syntax.OpCharClass:
		return insiemeRune{intervalli: append([]rune(nil), re.Rune...)}, false
	case syntax.OpCapture, syntax.OpPlus:
		return primeRune(re.Sub[0])
	case syntax.OpStar, syntax.OpQuest:
		s, _ := primeRune(re.Sub[0])
		return s, true
	case syntax.OpRepeat:
		s, vuota := primeRune(re.Sub[0])
		return s, vuota || re.Min == 0
	case syntax.OpConcat:
		var s insiemeRune
		for _, sub := range re.Sub {
			ss, vuota := primeRune(sub)
			s.unisci(ss)
			if !vuota {
				return s, false
			}
		}
		return s, true
	case syntax.OpAlternate:
		var s insiemeRune
		vuota := false
		for _, sub := range re.Sub {
			ss, v := primeRune(sub)
			s.unisci(ss)
			vuota = vuota || v
		}
		return s, vuota
	}
	return insiemeRune{tutte: true}, true
}
