// Package lettura prepara il corpo di una mail per essere LETTO in schermata.
//
// Il corpo che arriva da Outlook è pieno di testo che nessuno ha scritto: l'avviso «questa mail
// viene dall'esterno», il suggerimento di Microsoft 365 sul primo contatto, l'invito Teams in fondo,
// tre paragrafi di clausola di riservatezza. E una tabella incollata da Excel, nel testo semplice, è
// una fila di celle separate da TAB che non si legge. Qui quel testo diventa una sequenza di blocchi:
// il rumore CHIUSO (mai tolto: resta dentro, a un clic), le tabelle come tabelle, la storia citata a
// parte.
//
// # L'invariante
//
// Il corpo memorizzato non cambia. Questa è solo presentazione: nessuna funzione del package scrive
// niente, e il testo originale resta sempre raggiungibile intatto in [Corpo.Originale]. Se una
// regola sbaglia il danno è un blocco chiuso di troppo o una tabella mancata, mai una parola persa:
// ogni riga del testo sta in esattamente un blocco, e ogni blocco porta con sé le sue righe grezze.
// Nel dubbio il testo resta testo.
//
// # Mai HTML della mail nella pagina
//
// Dal corpo HTML si prende solo TESTO (le celle delle tabelle, o le righe quando il testo semplice
// manca). Tutto quello che esce da qui è testo semplice, da scrivere con l'escape come qualunque
// altra stringa: script, stili, attributi e marcatori non arrivano mai nel modello.
package lettura

import (
	"strings"
	"unicode/utf8"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// I limiti. Larghi per la posta vera, stretti per quella costruita apposta: un mega di testo non è
// una mail da leggere, è un allegato incollato, e si mostra com'è.
const (
	LimiteTesto = 1 << 20 // oltre: nessuna elaborazione, solo Originale (Ridotto)
	LimiteHTML  = 2 << 20 // oltre: l'HTML non si legge, restano il testo e il fallback TAB

	maxTabelle      = 50       // tabelle di dati per messaggio
	maxRigheTabella = 200      // righe non vuote
	maxColonne      = 30       // colonne effettive
	maxTestoTabella = 64 << 10 // testo totale delle celle
	maxRuneCella    = 500      // oltre, la cella si mostra troncata (il confronto usa il testo intero)
)

// Corpo è il corpo di un messaggio pronto per la schermata.
type Corpo struct {
	Blocchi    []Blocco // quello scritto adesso, in ordine
	Storia     []Blocco // la storia citata (classificazione.TagliaCatena), da mostrare chiusa
	Originale  string   // corpo_testo com'è nel DB: il «testo originale» sempre raggiungibile
	NRumore    int      // pezzi di testo automatico riconosciuti e chiusi (prima dell'unione), in Blocchi e Storia
	SoloRumore bool     // Blocchi non contiene né testo né tabelle, solo rumore (e separatori)
	DaHTML     bool     // il testo semplice era vuoto: i blocchi vengono dalla lettura dell'HTML
	Ridotto    bool     // testo oltre LimiteTesto (o errore interno): nessun blocco, solo Originale
}

// TipoBlocco dice come si mostra un blocco.
type TipoBlocco string

const (
	BloccoTesto      TipoBlocco = "testo"      // Pezzi, a capo conservati
	BloccoTabella    TipoBlocco = "tabella"    // Tabella
	BloccoRumore     TipoBlocco = "rumore"     // chiuso: Etichetta fuori, Grezzo dentro
	BloccoSeparatore TipoBlocco = "separatore" // una riga di ____ / ---- / ====
)

// Blocco è un tratto contiguo di righe di una sezione.
//
// Da e A sono le righe [Da,A) della sezione normalizzata a cui il blocco appartiene: la parte
// fresca per Corpo.Blocchi, la storia citata per Corpo.Storia (le due numerazioni sono separate).
// I blocchi di una sezione coprono tutte le sue righe, senza buchi né sovrapposizioni.
type Blocco struct {
	Tipo      TipoBlocco
	Pezzi     []Pezzo  // solo per BloccoTesto
	Tabella   *Tabella // solo per BloccoTabella
	Motivi    []Motivo // solo per BloccoRumore, distinti, nell'ordine in cui compaiono
	Etichetta string   // solo per BloccoRumore: una riga, da mettere nel <summary>
	Grezzo    string   // le righe [Da,A) come sono, senza le righe vuote ai bordi
	Da, A     int
}

// TipoPezzo dice come si mostra un pezzo di un blocco di testo.
type TipoPezzo string

const (
	PezzoTesto    TipoPezzo = "testo"    // testo semplice
	PezzoLink     TipoPezzo = "link"     // un indirizzo riscritto o annotato: in grigio, MAI cliccabile
	PezzoImmagine TipoPezzo = "immagine" // il segnaposto «[immagine]»; Nota = nome o testo alternativo
)

// Pezzo è un tratto di testo di un blocco. Testo e Nota sono testo semplice.
type Pezzo struct {
	Tipo  TipoPezzo
	Testo string
	Nota  string // per il title: perché il pezzo è stato riscritto, o che immagine era
}

// Motivo è la regola che ha riconosciuto un pezzo di rumore.
type Motivo string

const (
	MotivoBannerEsterno Motivo = "banner_esterno"
	MotivoPrimoContatto Motivo = "primo_contatto"
	MotivoOutlookMobile Motivo = "outlook_mobile"
	MotivoRiunioneTeams Motivo = "riunione_teams"
	MotivoRiservatezza  Motivo = "riservatezza"
	MotivoAmbiente      Motivo = "ambiente"
)

// Origine di una tabella.
const (
	OrigineHTML = "html" // dal corpo HTML, ancorata nel testo
	OrigineTab  = "tab"  // righe di testo con le celle separate da TAB
)

// Tabella è una tabella da mostrare come tabella.
type Tabella struct {
	Righe        []RigaTabella
	Colonne      int
	Intestazione bool   // la riga 0 è l'intestazione (mai per le tabelle «tab»)
	Origine      string // OrigineHTML | OrigineTab
}

// RigaTabella è una riga di una tabella.
type RigaTabella struct{ Celle []Cella }

// Cella è una cella di una tabella. Testo è testo semplice, con \n dove la cella andava a capo.
type Cella struct {
	Testo            string
	Colspan, Rowspan int  // sempre ≥ 1, già limitati alla tabella
	Numero           bool // da allineare a destra
	Troncata         bool // Testo è tagliato a 500 rune
}

// Presenta trasforma il corpo di un messaggio (corpo_testo e corpo_html) nei blocchi da mostrare.
//
// È deterministica, non scrive niente e non va mai in panic. L'ultima difesa è un recover che
// ripiega su Corpo{Originale: testo, Ridotto: true}: non è il modo in cui si gestiscono gli errori
// (il codice sotto non ne deve avere), è la garanzia che un difetto qui dentro costi al più la vista
// pulita di un messaggio e mai la pagina intera.
func Presenta(testo, html string) (c Corpo) {
	defer func() {
		if r := recover(); r != nil {
			c = Corpo{Originale: testo, Ridotto: true}
		}
	}()
	c, _ = elabora(testo, html)
	return c
}

// lavoro sono le due sezioni dopo l'elaborazione: le prove le usano per controllare la copertura.
type lavoro struct {
	fresca, citata *sezione
}

// elabora è Presenta senza la rete di sicurezza: le prove (e il fuzz) chiamano questa, così un
// panic si vede invece di diventare un Ridotto silenzioso.
func elabora(testo, h string) (Corpo, *lavoro) {
	c := Corpo{Originale: testo}
	if len(testo) > LimiteTesto {
		c.Ridotto = true
		return c, nil
	}
	t := fineRiga(testo)
	vuoto := strings.TrimSpace(t) == ""

	// L'HTML si legge solo se serve: per le tabelle quando ce n'è almeno una, per tutto quando il
	// testo semplice manca. La gran parte della posta non ha tabelle e non paga il parser.
	var doc *documentoHTML
	if h != "" && len(h) <= LimiteHTML && (vuoto || contieneTabella(h)) {
		doc = leggiHTML(h)
	}
	if vuoto {
		if doc == nil {
			return c, nil
		}
		t = doc.testo()
		c.DaHTML = true
		if strings.TrimSpace(t) == "" {
			return c, nil
		}
		if len(t) > LimiteTesto {
			c.Ridotto = true
			return c, nil
		}
	}

	utile, storia := classificazione.TagliaCatena(t)
	l := &lavoro{fresca: nuovaSezione(utile), citata: nuovaSezione(storia)}
	if doc != nil {
		ancora(doc.tabelle, l.fresca, l.citata)
	}
	for _, s := range []*sezione{l.fresca, l.citata} {
		s.rumore()
		s.separatori()
		s.tabelleTab()
	}
	c.Blocchi = l.fresca.blocchi()
	c.Storia = l.citata.blocchi()
	c.NRumore = l.fresca.nRumore + l.citata.nRumore
	c.SoloRumore = soloRumore(c.Blocchi)
	return c, l
}

func soloRumore(bb []Blocco) bool {
	rumore := false
	for _, b := range bb {
		switch b.Tipo {
		case BloccoRumore:
			rumore = true
		case BloccoTesto, BloccoTabella:
			return false
		}
	}
	return rumore
}

// ---------------------------------------------------------------- normalizzazione

// fineRiga porta tutti i fine riga a \n: le righe si contano su questo.
func fineRiga(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// perConfronto è la forma su cui girano le regole, MAI quella che si mostra: gli spazi unificatori
// diventano spazi, i caratteri invisibili spariscono, l'apostrofo tipografico diventa «'». Senza,
// «dall’esterno» non è «dall'esterno» e un avviso scritto da Word non si riconosce.
func perConfronto(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u00a0', '\u2007', '\u202f':
			return ' '
		case '\u200b', '\u200c', '\u200d', '\ufeff', '\u00ad':
			return -1
		case '\u2019':
			return '\''
		}
		return r
	}, s)
}

// perVista è la sola normalizzazione che arriva in schermata (I6): gli spazi unificatori si
// mostrano come spazi. Il resto del testo è quello del messaggio.
func perVista(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u00a0', '\u2007', '\u202f':
			return ' '
		}
		return r
	}, s)
}

// tronca taglia s a n rune.
func tronca(s string, n int) (string, bool) {
	if utf8.RuneCountInString(s) <= n {
		return s, false
	}
	i := 0
	for k := range s {
		if i == n {
			return s[:k], true
		}
		i++
	}
	return s, false
}

// ---------------------------------------------------------------- la sezione

// sezione è la parte fresca o la parte citata, divisa in righe. Ogni riga ha al più un
// proprietario (una regione: tabella, rumore, separatore); le righe libere alla fine diventano
// testo. È così che la copertura senza buchi né sovrapposizioni è garantita per costruzione.
type sezione struct {
	grezze  []string // le righe come sono
	norm    []string // perConfronto + TrimSpace: la forma delle regole
	prop    []int    // -1 libera, altrimenti indice in regioni
	regioni []regione
	nRumore int
}

type regione struct {
	tipo      TipoBlocco
	da, a     int
	motivo    Motivo
	etichetta string
	tabella   *Tabella
}

func nuovaSezione(testo string) *sezione {
	s := &sezione{}
	if testo == "" {
		return s
	}
	s.grezze = strings.Split(testo, "\n")
	s.norm = make([]string, len(s.grezze))
	s.prop = make([]int, len(s.grezze))
	for i, r := range s.grezze {
		s.norm[i] = strings.TrimSpace(perConfronto(r))
		s.prop[i] = -1
	}
	return s
}

func (s *sezione) libera(i int) bool { return i >= 0 && i < len(s.prop) && s.prop[i] < 0 }

func (s *sezione) vuota(i int) bool { return s.norm[i] == "" }

// occupa assegna le righe [da,a) a una regione, solo se sono tutte libere.
func (s *sezione) occupa(r regione) bool {
	if r.da < 0 || r.a > len(s.prop) || r.da >= r.a {
		return false
	}
	for i := r.da; i < r.a; i++ {
		if s.prop[i] >= 0 {
			return false
		}
	}
	k := len(s.regioni)
	s.regioni = append(s.regioni, r)
	for i := r.da; i < r.a; i++ {
		s.prop[i] = k
	}
	if r.tipo == BloccoRumore {
		s.nRumore++
	}
	return true
}

// paragrafi sono le sequenze massimali di righe libere non vuote, come [da,a).
func (s *sezione) paragrafi() [][2]int {
	var out [][2]int
	for i := 0; i < len(s.norm); {
		if !s.libera(i) || s.vuota(i) {
			i++
			continue
		}
		j := i
		for j < len(s.norm) && s.libera(j) && !s.vuota(j) {
			j++
		}
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

// testoNorm sono le righe [da,a) nella forma delle regole, unite da \n.
func (s *sezione) testoNorm(da, a int) string {
	return strings.Join(s.norm[da:a], "\n")
}

// grezzo sono le righe [da,a) come sono, senza le righe vuote ai bordi.
func (s *sezione) grezzo(da, a int) string {
	for da < a && strings.TrimSpace(s.grezze[da]) == "" {
		da++
	}
	for a > da && strings.TrimSpace(s.grezze[a-1]) == "" {
		a--
	}
	return strings.Join(s.grezze[da:a], "\n")
}

// ---------------------------------------------------------------- i blocchi

// blocchi percorre le righe in ordine e ne fa blocchi: ogni regione il suo, ogni tratto di righe
// libere un blocco di testo. Un tratto libero fatto solo di righe vuote non diventa un blocco di
// testo vuoto: si attacca al blocco prima (o a quello dopo, se è in cima).
func (s *sezione) blocchi() []Blocco {
	var out []Blocco
	n := len(s.grezze)
	sospese := -1 // righe vuote in cima, da dare al primo blocco
	for i := 0; i < n; {
		var b Blocco
		if k := s.prop[i]; k >= 0 {
			b = s.bloccoRegione(s.regioni[k])
		} else {
			j := i
			for j < n && s.prop[j] < 0 {
				j++
			}
			if s.tuttoVuoto(i, j) {
				if len(out) > 0 {
					out[len(out)-1].A = j
				} else {
					sospese = i
				}
				i = j
				continue
			}
			b = s.bloccoTesto(i, j)
		}
		if sospese >= 0 {
			b.Da = sospese
			sospese = -1
		}
		out = append(out, b)
		i = b.A
	}
	return s.unisciRumore(out)
}

func (s *sezione) tuttoVuoto(da, a int) bool {
	for i := da; i < a; i++ {
		if !s.vuota(i) {
			return false
		}
	}
	return true
}

func (s *sezione) bloccoRegione(r regione) Blocco {
	b := Blocco{Tipo: r.tipo, Da: r.da, A: r.a, Grezzo: s.grezzo(r.da, r.a)}
	switch r.tipo {
	case BloccoTabella:
		b.Tabella = r.tabella
	case BloccoRumore:
		b.Motivi = []Motivo{r.motivo}
		b.Etichetta = r.etichetta
	}
	return b
}

func (s *sezione) bloccoTesto(da, a int) Blocco {
	return Blocco{
		Tipo:   BloccoTesto,
		Pezzi:  pezziDaRighe(righeVista(s.grezze[da:a])),
		Grezzo: s.grezzo(da, a),
		Da:     da,
		A:      a,
	}
}

// unisciRumore unisce i blocchi di rumore che si toccano: un avviso di posta esterna seguito dal
// suggerimento del primo contatto sono due righe di testo automatico, non due cose da aprire.
func (s *sezione) unisciRumore(bb []Blocco) []Blocco {
	var out []Blocco
	var unito []bool
	for _, b := range bb {
		if k := len(out) - 1; k >= 0 && b.Tipo == BloccoRumore && out[k].Tipo == BloccoRumore && out[k].A == b.Da {
			u := &out[k]
			u.A = b.A
			for _, m := range b.Motivi {
				if !contieneMotivo(u.Motivi, m) {
					u.Motivi = append(u.Motivi, m)
				}
			}
			// Due pezzi dello stesso motivo (la riga «[EXTERNAL]» e l'avviso sotto) tengono
			// l'etichetta del primo: dire «testo automatico: posta esterna» non aggiunge niente.
			if len(u.Motivi) > 1 {
				u.Etichetta = etichettaUnita(u.Motivi)
			}
			unito[k] = true
			continue
		}
		out = append(out, b)
		unito = append(unito, false)
	}
	// Il testo grezzo si ricompone una volta sola alla fine: ricomporlo a ogni unione costerebbe il
	// quadrato delle righe su una mail fatta di mille firme «Inviato da iPhone» una sotto l'altra.
	for k := range out {
		if unito[k] {
			out[k].Grezzo = s.grezzo(out[k].Da, out[k].A)
		}
	}
	return out
}

func contieneMotivo(mm []Motivo, m Motivo) bool {
	for _, x := range mm {
		if x == m {
			return true
		}
	}
	return false
}

// righeVista applica I6 alle righe di un blocco di testo: spazi finali via, spazi unificatori come
// spazi, due o più righe vuote diventano una, niente righe vuote ai bordi.
func righeVista(righe []string) []string {
	out := make([]string, 0, len(righe))
	vuote := 0
	for _, r := range righe {
		r = strings.TrimRight(perVista(r), " \t")
		if strings.TrimSpace(r) == "" {
			r = ""
			vuote++
			if vuote > 1 {
				continue
			}
		} else {
			vuote = 0
		}
		out = append(out, r)
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}
