package motorea

import (
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// LetturaForma: ciò che una forma riconosce in un testo, prima del router. È il risultato, non il runtime: i
// campi seguono gli attesi e R16, e li riempiono le operazioni dei gruppi del piano (par.12). Non ha funzione
// né ruoli: li assegna Interpreta, in A1b (R23).
type LetturaForma struct {
	Famiglia, Namespace, Forma string
	Selettore                  evidenze.Selettore
	Originale                  string              // il testo letto, per intero: testo[Inizio:Fine]
	Intervallo                 evidenze.Intervallo // in byte, sul testo dato
	CodiceRichiesto            string              // affissi e base, senza etichetta, decorazioni, token e revisione
	Base                       BaseLetta
	Marcatore                  *ParteLetta        // il marcatore dopo la base (D1, R7)
	Token                      *ParteLetta        // la parte token (R16): conservata e mai attribuita (Q1, D10)
	Etichetta                  *ParteLetta        // A-C08
	Affissi                    []AffissoLetto     // nell'ordine del testo
	Revisione                  *RevisioneLetta    // nil se la forma non ne legge una
	Decorazioni                []DecorazioneLetta // mai dentro la base
	Ripetizioni                []RipetizioneLetta // la base ripetuta: concordante o discordante (D2)
	Categorie                  []grammatica.Categoria
	Stato                      string // completa | parziale | discordante | da_verificare
}

// Gli stati di una lettura. Se ne valgono più d'uno, vince il più forte: discordante, poi da_verificare, poi
// parziale (la base parziale resta comunque in Base.Completa).
const (
	StatoCompleta     = "completa"
	StatoParziale     = "parziale"      // una forma parziale: vincoli, non un articolo (A-C07)
	StatoDiscordante  = "discordante"   // una ripetizione della base non concorda: nessuna scelta della prima (D2)
	StatoDaVerificare = "da_verificare" // la revisione è un token sospeso: la base è una candidata (D5)
)

// BaseLetta: l'originale, i segmenti osservati e quelli mancanti (parte 1 §5.1). Senza tutti i segmenti
// identitari non esiste una chiave completa: una forma parziale dà vincoli, non un articolo (A-C07). I
// segmenti mancanti non hanno nessun SegmentoLetto: niente completamento inventato.
type BaseLetta struct {
	Originale, Normalizzata string
	Segmenti                []SegmentoLetto
	Mancanti                []string
	Completa                bool
}

// SegmentoLetto: un segmento nominato della base o della revisione, con il suo intervallo sul testo dato.
type SegmentoLetto struct {
	Nome, Originale, Normalizzato string
	Intervallo                    evidenze.Intervallo
}

// ParteLetta: una parte conservata com'è (marcatore, token, etichetta). Per l'etichetta Valore è il letterale
// e Originale comprende gli spazi che lo seguono.
type ParteLetta struct {
	Valore, Originale string
	Intervallo        evidenze.Intervallo
}

// AffissoLetto: Attribuito falso vuol dire riconosciuto ma senza significato (D5). Valore c'è solo se
// l'affisso è attribuito sul selettore della lettura (parte 1 §5.3).
type AffissoLetto struct {
	Regola, Originale, Posizione string
	Intervallo                   evidenze.Intervallo
	Attribuito                   bool
	Valore                       *grammatica.ValoreQualificatore
}

// RevisioneLetta: la stringa resta com'è, zeri compresi (D4).
//   - Stati della parte 1 §5.2: assente | letta | ambigua | discordante | non_interpretabile. In A1a il
//     motore ne produce due: letta (i segmenti) e non_interpretabile (un token sospeso); una revisione assente
//     è una RevisioneLetta nil.
//   - Originale comprende il separatore; Normalizzata è la stringa dei segmenti, vuota se lo stato non è
//     «letta».
//   - Segmenti nominati e Token sono le estensioni di D4/D5 (R16).
type RevisioneLetta struct {
	Regola, Originale, Normalizzata, Stato, Sorgente string
	Segmenti                                         []SegmentoLetto // forte, debole
	Token                                            *ParteLetta     // «xx», «30»: conservato, non attribuito
	RichiedeVerifica                                 bool
	Intervallo                                       evidenze.Intervallo

	// equivalenze: le coppie dichiarate dalla regola (parte 1 §5.2: solo dichiarate). Le legge
	// ConfrontaRevisioni; restano nil se la regola non ne dichiara, come tutte le grammatiche di A1.
	equivalenze [][2]string
}

// Gli stati della revisione che il motore produce in A1a.
const (
	StatoRevisioneLetta             = "letta"
	StatoRevisioneNonInterpretabile = "non_interpretabile"
)

// DecorazioneLetta: una parte accessoria, fuori dall'identità. Tipo è l'etichetta della decorazione
// dichiarata (involucro, livello_nome_file, stato_pdm, suffisso_documento, descrizione): il motore guarda solo
// letterali, pattern e parti (par.12).
type DecorazioneLetta struct {
	Regola, Tipo, Valore, Originale, Riserva string
	Intervallo                               evidenze.Intervallo
}

// RipetizioneLetta: la base scritta una seconda volta (D2). Concorda se la sua forma normalizzata è uguale a
// quella della base: il confronto si fa in Go, perché RE2 non ha backreference (T19).
type RipetizioneLetta struct {
	Base     BaseLetta
	Concorda bool
}

// lettura costruisce una LetturaForma dai gruppi di un match. loc sono gli indici del match su testo[inizio:]:
// ogni gruppo porta la sua operazione, e qui la si applica (par.12: conservare, normalizzare, attribuire,
// confrontare con la base).
func (p *pianoForma) lettura(sel evidenze.Selettore, testo string, inizio int, loc []int) LetturaForma {
	l := LetturaForma{
		Famiglia:  p.famiglia,
		Namespace: p.namespace,
		Forma:     p.forma,
		Selettore: sel,
		Categorie: copiaCategorie(p.categorie),
	}
	m := match{testo: testo, inizio: inizio, loc: loc}
	var codice []string
	for i, g := range p.gruppi {
		a, b, ok := m.pos(g)
		if !ok {
			continue
		}
		iv := evidenze.Intervallo{Inizio: a, Fine: b}
		switch g.op {
		case opLettura:
			l.Originale, l.Intervallo = testo[a:b], iv
		case opBase:
			l.Base = p.baseLetta(m, i, a, b)
			codice = append(codice, l.Base.Originale)
		case opRipetizione:
			l.Ripetizioni = append(l.Ripetizioni, RipetizioneLetta{Base: p.baseLetta(m, i, a, b)})
		case opEtichetta:
			if l.Etichetta == nil {
				l.Etichetta = &ParteLetta{Valore: m.figlio(p.gruppi, i, opValoreEtichetta), Originale: testo[a:b], Intervallo: iv}
			}
		case opAffisso:
			af := AffissoLetto{Regola: g.regola, Originale: testo[a:b], Posizione: g.posizione, Intervallo: iv,
				Attribuito: contiene(g.attribuzione, sel.String())}
			if af.Attribuito && g.valore != nil {
				v := *g.valore
				af.Valore = &v
			}
			l.Affissi = append(l.Affissi, af)
			codice = append(codice, af.Originale)
		case opMarcatore:
			if l.Marcatore == nil {
				l.Marcatore = &ParteLetta{Valore: testo[a:b], Originale: testo[a:b], Intervallo: iv}
			}
		case opToken:
			if l.Token == nil {
				l.Token = &ParteLetta{Valore: testo[a:b], Originale: testo[a:b], Intervallo: iv}
			}
		case opDecorazione:
			l.Decorazioni = append(l.Decorazioni, DecorazioneLetta{Regola: g.regola, Tipo: g.tipo, Valore: testo[a:b],
				Originale: testo[a:b], Riserva: g.riserva, Intervallo: iv})
		case opRevisione:
			if l.Revisione == nil {
				l.Revisione = revisioneLetta(p.gruppi, m, i, g.rev, a, b)
			}
		}
	}
	// Il codice richiesto: affissi e base, nell'ordine del testo (i gruppi seguono l'ordine delle parti).
	for _, s := range codice {
		l.CodiceRichiesto += s
	}
	// Le ripetizioni si confrontano con la base, normalizzate: nessuna scelta della prima (D2, T19).
	discordante := false
	for i := range l.Ripetizioni {
		l.Ripetizioni[i].Concorda = l.Ripetizioni[i].Base.Normalizzata == l.Base.Normalizzata
		if !l.Ripetizioni[i].Concorda {
			discordante = true
		}
	}
	switch {
	case discordante:
		l.Stato = StatoDiscordante
	case l.Revisione != nil && l.Revisione.Stato == StatoRevisioneNonInterpretabile:
		l.Stato = StatoDaVerificare
	case !p.completa:
		l.Stato = StatoParziale
	default:
		l.Stato = StatoCompleta
	}
	return l
}

// match: un match di un piano, con gli indici relativi a testo[inizio:].
type match struct {
	testo  string
	inizio int
	loc    []int
}

// pos: l'intervallo assoluto di un gruppo, se ha partecipato al match.
func (m match) pos(g gruppo) (int, int, bool) {
	a := m.loc[2*g.numero]
	if a < 0 {
		return 0, 0, false
	}
	return m.inizio + a, m.inizio + m.loc[2*g.numero+1], true
}

// figlio: il testo del primo gruppo con quell'operazione dentro il gruppo padre.
func (m match) figlio(gruppi []gruppo, padre int, op operazione) string {
	for _, g := range gruppi {
		if g.padre == padre && g.op == op {
			if a, b, ok := m.pos(g); ok {
				return m.testo[a:b]
			}
		}
	}
	return ""
}

// baseLetta: la base (o la sua ripetizione) del gruppo i, con i segmenti che contiene.
func (p *pianoForma) baseLetta(m match, i, a, b int) BaseLetta {
	bl := BaseLetta{
		Originale:    m.testo[a:b],
		Normalizzata: p.normalizza(m.testo[a:b]),
		Mancanti:     copiaStringhe(p.mancanti),
		Completa:     p.completa,
	}
	for _, g := range p.gruppi {
		if g.padre != i || g.op != opSegmento {
			continue
		}
		if sa, sb, ok := m.pos(g); ok {
			bl.Segmenti = append(bl.Segmenti, SegmentoLetto{Nome: g.nome, Originale: m.testo[sa:sb],
				Normalizzato: p.normalizza(m.testo[sa:sb]), Intervallo: evidenze.Intervallo{Inizio: sa, Fine: sb}})
		}
	}
	return bl
}

// normalizza applica la normalizzazione dichiarata della base: nessuna, o le maiuscole ASCII. Le lettere fuori
// dall'ASCII non cambiano, così la lunghezza in byte resta quella dell'originale (T9).
func (p *pianoForma) normalizza(s string) string {
	if !p.maiuscolo {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// revisioneLetta: la revisione del gruppo i, inline in una forma o in campo separato (D-07: la stessa
// funzione per le due). Se il testo dopo il separatore lo legge un token sospeso, la revisione è il token:
// conservato, nessun valore, da verificare (D5). Il token vince anche quando i segmenti leggerebbero lo
// stesso testo: con due letture possibili la scelta prudente è non attribuire un valore.
func revisioneLetta(gruppi []gruppo, m match, i int, r *regolaRevisione, a, b int) *RevisioneLetta {
	rl := &RevisioneLetta{
		Regola:      r.id,
		Originale:   m.testo[a:b],
		Sorgente:    r.sorgente,
		Intervallo:  evidenze.Intervallo{Inizio: a, Fine: b},
		equivalenze: r.equivalenze,
	}
	valore, token := -1, -1
	for k, g := range gruppi {
		if g.padre != i {
			continue
		}
		if _, _, ok := m.pos(g); !ok {
			continue
		}
		switch g.op {
		case opValoreRevisione:
			valore = k
		case opTokenRevisione:
			token = k
		}
	}
	if token < 0 && valore >= 0 && r.reToken != nil {
		va, vb, _ := m.pos(gruppi[valore])
		if r.reToken.MatchString(m.testo[va:vb]) {
			token, valore = valore, -1
		}
	}
	switch {
	case token >= 0:
		ta, tb, _ := m.pos(gruppi[token])
		rl.Stato = StatoRevisioneNonInterpretabile
		rl.Token = &ParteLetta{Valore: m.testo[ta:tb], Originale: m.testo[ta:tb], Intervallo: evidenze.Intervallo{Inizio: ta, Fine: tb}}
		rl.RichiedeVerifica = true
	case valore >= 0:
		va, vb, _ := m.pos(gruppi[valore])
		rl.Stato = StatoRevisioneLetta
		rl.Normalizzata = m.testo[va:vb]
		for _, g := range gruppi {
			if g.padre != valore || g.op != opSegmentoRevisione {
				continue
			}
			if sa, sb, ok := m.pos(g); ok {
				rl.Segmenti = append(rl.Segmenti, SegmentoLetto{Nome: g.nome, Originale: m.testo[sa:sb],
					Normalizzato: m.testo[sa:sb], Intervallo: evidenze.Intervallo{Inizio: sa, Fine: sb}})
			}
		}
	}
	return rl
}

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}

// copiaStringhe e copiaCategorie: lo stesso elenco, nil se vuoto (vuoto e assente danno lo stesso canonico,
// par.3.4.3).
func copiaStringhe(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return append([]string(nil), s...)
}

func copiaCategorie(c []grammatica.Categoria) []grammatica.Categoria {
	if len(c) == 0 {
		return nil
	}
	return append([]grammatica.Categoria(nil), c...)
}
