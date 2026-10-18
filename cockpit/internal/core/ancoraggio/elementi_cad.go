package ancoraggio

import (
	"regexp"
	"slices"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Il segnale LS-02 (V1: emendamento di V1 §11.1; addendum ad A1c §2.2 del 18/10, «il segnale LS-02»): un nodo di uno
// STEP che ha la forma di un elemento CAD, un cordone di saldatura o simile, e non di un articolo. È solo un segnale,
// spiegato nodo per nodo (direttiva 23 §4.2): non toglie il nodo, non lo esenta dal 2D, non cambia tipo, classe,
// perimetro, righe da decidere, Leggibile né il documento delle evidenze, non decide e non scrive niente. La decisione
// di non includere un nodo nella BOM la prende solo l'ingegnere, nella bozza, e la applica la conferma.
//
// Nasce qui, dove nasce il NodoProposto di uno STEP (struttura, l'unico punto): la pagina lo riceve tale e quale,
// perché valutazione.NodoBOM incorpora il nodo, e l'adattatore del registro d'importazione lo copia. Una regola sola,
// nessuna copia.

// Il codice del segnale (SegnaleNodo.Codice) è CodiceSegnaleElementoCAD, in codici_diagnostica.go come ogni codice che
// il pacchetto produce (R41 b; decisione P-1 (a) dell'ondata A di V1), con la riga dell'elenco d'oro e l'etichetta della
// pagina.

// RegolaElementoCADNomeFogliaV1: la regola che dà il segnale, con la versione nel nome. Una parola nuova o un criterio
// nuovo vogliono una misura e una regola _v2, mai un ritocco di questa.
const RegolaElementoCADNomeFogliaV1 = "elemento_cad_nome_foglia_v1"

// I contro-indizi di un segnale (SegnaleNodo.ControIndizi): codici, non testo libero, nell'ordine qui sotto. Non
// cambiano l'esito: lo dicono nella spiegazione.
//   - lettura_identita: il nodo ha anche una lettura d'identità (Leggibile): potrebbe essere un pezzo vero;
//   - grafo_non_completo: il grafo del file non è completo (StrutturaFile.GrafoCompleto falso): che il nodo non abbia
//     figli potrebbe non essere un'informazione; la spiegazione riporta il motivo (StrutturaFile.MotivoGrafo).
const (
	ControIndizioLetturaIdentita  = "lettura_identita"
	ControIndizioGrafoNonCompleto = "grafo_non_completo"
)

// EspressioneElementoCADV1: l'espressione della regola v1 (RE2), la stessa dell'emendamento di V1 §11.1. È la
// traduzione dell'espressione Python della misura del 17/10, che RE2 non può scrivere tale e quale perché non ha il
// lookbehind: i confini sono scritti a mano, con le maiuscole dichiarate. Le due espressioni danno lo stesso esito sui
// testi delle prove (Q-01); fuori dall'ASCII differiscono solo sulle due i turche, İ e ı, che Python con re.I tratta
// come «i» e RE2 no (limite dichiarato nell'addendum). Vale questa.
const EspressioneElementoCADV1 = `(?i)(?:^|[^A-Za-z])(?:saldatura|cordone|fillet|bead)(?:$|[^A-Za-z])|(?:^|[^A-Za-z])weld[ _-]*(?:bead|seam|id)(?:$|[^A-Za-z])`

var (
	// espressioneElementoCADV1 decide se un testo contiene una parola della regola.
	espressioneElementoCADV1 = regexp.MustCompile(EspressioneElementoCADV1)
	// paroleElementoCADV1 è la stessa espressione con le due alternative raccolte in un gruppo, che dà la parola senza
	// i confini: serve solo a dire dove sono le parole (RiscontroNome), mai a decidere.
	paroleElementoCADV1 = regexp.MustCompile(`(?i)(?:^|[^A-Za-z])((?:saldatura|cordone|fillet|bead)|weld[ _-]*(?:bead|seam|id))(?:$|[^A-Za-z])`)
)

// SegnaleNodo: un segnale su un nodo di uno STEP (V1, LS-02; addendum ad A1c §2.2 del 18/10). Un solo segnale per
// regola, con un riscontro per ogni campo grezzo che ha la parola.
//   - Codice: CodiceSegnaleElementoCAD. Regola: RegolaElementoCADNomeFogliaV1.
//   - Riscontri: i campi grezzi del PRODUCT con le parole della regola, nell'ordine id, nome (NodoStruttura.Grezzi).
//   - ControIndizi: ControIndizio*, nell'ordine delle costanti; vuoto senza.
//   - Spiegazione: il testo per l'ingegnere, composto in modo deterministico dagli altri campi (spiegazioneElementoCAD).
//
// Foglia e «non radice» sono sempre veri per un nodo segnalato: non sono campi.
type SegnaleNodo struct {
	Codice       string          `json:"codice"`
	Regola       string          `json:"regola"`
	Riscontri    []RiscontroNome `json:"riscontri"`
	ControIndizi []string        `json:"contro_indizi,omitempty"`
	Spiegazione  string          `json:"spiegazione"`
}

// RiscontroNome: dove un campo grezzo del PRODUCT ha le parole della regola.
//   - Campo: id | nome (CampoGrezzoID, CampoGrezzoNome). UnitaID: l'unità del grezzo nel documento delle evidenze.
//   - Parole: le parole com'erano nel testo («Saldatura», «WELD_ID»), nell'ordine del testo.
//   - Intervalli: per ogni parola il suo intervallo [Inizio, Fine) in byte sul testo grezzo, senza i confini.
type RiscontroNome struct {
	Campo      string                `json:"campo"`
	UnitaID    string                `json:"unita_id"`
	Parole     []string              `json:"parole"`
	Intervalli []evidenze.Intervallo `json:"intervalli"`
}

// segnaliElementoCAD: i segnali LS-02 di un nodo di uno STEP, con la regola elemento_cad_nome_foglia_v1. Il nodo è
// segnalato se, e solo se, sono vere tutte e tre le condizioni:
//  1. l'id o il nome grezzo del PRODUCT contiene, come parola, saldatura, cordone, fillet o bead, oppure weld seguito
//     da bead, seam o id, con in mezzo spazi, «_» o «-» (EspressioneElementoCADV1);
//  2. il nodo è una foglia nel file: nessun arco dei fatti parte dal nodo (foglia);
//  3. il nodo non è una radice del file (radice).
//
// Allora c'è un segnale solo, con un riscontro per ogni campo grezzo che ha la parola; altrimenti nil. La lettura
// d'identità del nodo e la completezza del grafo del file (grafoCompleto, con il motivo motivoGrafo che la spiegazione
// riporta) sono solo contro-indizi: non cambiano l'esito. La geometria, il nome d'istanza e l'assenza del 2D non sono ingressi: non arrivano al documento, e la
// direttiva 23 vieta l'assenza di geometria come criterio universale. È pura: gli elenchi dell'uscita sono nuovi.
func segnaliElementoCAD(n NodoStruttura, foglia, radice, grafoCompleto bool, motivoGrafo string) []SegnaleNodo {
	if !foglia || radice {
		return nil
	}
	var riscontri []RiscontroNome
	for _, g := range n.Grezzi {
		if g.Campo != CampoGrezzoID && g.Campo != CampoGrezzoNome {
			continue
		}
		if !espressioneElementoCADV1.MatchString(g.Testo) {
			continue
		}
		parole, intervalli := paroleDelGrezzo(g.Testo)
		riscontri = append(riscontri, RiscontroNome{Campo: g.Campo, UnitaID: g.UnitaID, Parole: parole, Intervalli: intervalli})
	}
	if len(riscontri) == 0 {
		return nil
	}
	s := SegnaleNodo{Codice: CodiceSegnaleElementoCAD, Regola: RegolaElementoCADNomeFogliaV1, Riscontri: riscontri}
	var letti []string
	if len(n.Letture) > 0 {
		s.ControIndizi = append(s.ControIndizi, ControIndizioLetturaIdentita)
		letti = codiciLetti(n)
	}
	if !grafoCompleto {
		s.ControIndizi = append(s.ControIndizi, ControIndizioGrafoNonCompleto)
	}
	s.Spiegazione = spiegazioneElementoCAD(riscontri, letti, !grafoCompleto, motivoGrafo)
	return []SegnaleNodo{s}
}

// paroleDelGrezzo: le parole della regola v1 in un testo grezzo, com'erano nel testo, con il loro intervallo in byte.
// Le cerca da sinistra e riparte dalla fine di ogni parola: il confine destro di una parola può essere il confine
// sinistro della successiva («FILLET_WELD_ID_3» dà FILLET e WELD_ID). Ripartendo da lì, l'inizio del testo che
// l'espressione vede è un carattere che non è una lettera, o la fine: nessuna parola nuova comincia senza il suo
// confine.
func paroleDelGrezzo(testo string) ([]string, []evidenze.Intervallo) {
	var parole []string
	var intervalli []evidenze.Intervallo
	for da := 0; da < len(testo); {
		m := paroleElementoCADV1.FindStringSubmatchIndex(testo[da:])
		if m == nil || m[2] < 0 {
			break
		}
		inizio, fine := da+m[2], da+m[3]
		parole = append(parole, testo[inizio:fine])
		intervalli = append(intervalli, evidenze.Intervallo{Inizio: inizio, Fine: fine})
		da = fine
	}
	return parole, intervalli
}

// codiciLetti: i letterali delle letture d'identità del nodo (LetturaForma.Originale), senza doppioni, nell'ordine
// delle letture (per ID).
func codiciLetti(n NodoStruttura) []string {
	var out []string
	for _, l := range n.Letture {
		if l.Forma.Originale != "" && !slices.Contains(out, l.Forma.Originale) {
			out = append(out, l.Forma.Originale)
		}
	}
	return out
}

// spiegazioneElementoCAD: la spiegazione del segnale, composta dai campi in modo deterministico. Prima dove sono le
// parole (l'id, il nome o tutti e due), poi le due condizioni e che cosa vuol dire il segnale, poi i contro-indizi. Le
// due frasi del segnale e del grafo sono quelle della decisione P-5 dell'ondata A di V1 (addendum ad A1c §2.2, rev. 3):
// vere anche per un nodo già scartato e per ogni motivo del grafo non completo, che la frase riporta tale e quale.
func spiegazioneElementoCAD(riscontri []RiscontroNome, letti []string, grafoNonCompleto bool, motivoGrafo string) string {
	var id, nome []string // le parole di ogni campo, senza doppioni, nell'ordine del testo
	for _, r := range riscontri {
		for _, p := range r.Parole {
			switch {
			case r.Campo == CampoGrezzoID && !slices.Contains(id, p):
				id = append(id, p)
			case r.Campo == CampoGrezzoNome && !slices.Contains(nome, p):
				nome = append(nome, p)
			}
		}
	}
	var b strings.Builder
	switch {
	case len(id) > 0 && len(nome) > 0 && slices.Equal(id, nome):
		b.WriteString("L'id e il nome del pezzo nel file contengono " + elencoParole(id))
	case len(id) > 0 && len(nome) > 0:
		b.WriteString("L'id del pezzo nel file contiene " + elencoParole(id) + " e il nome " + elencoParole(nome))
	case len(id) > 0:
		b.WriteString("L'id del pezzo nel file contiene " + elencoParole(id))
	default:
		b.WriteString("Il nome del pezzo nel file contiene " + elencoParole(nome))
	}
	b.WriteString("; il nodo non ha figli e non è una radice del file: possibile elemento CAD (un cordone di saldatura o simile), " +
		"non un articolo. È solo un segnale: da solo non toglie il nodo dalla proposta né il suo 2D, e la decisione è tua.")
	switch len(letti) {
	case 0:
	case 1:
		b.WriteString(" Ha anche un codice letto (" + letti[0] + "): controlla che non sia un pezzo vero.")
	default:
		b.WriteString(" Ha anche dei codici letti (" + strings.Join(letti, ", ") + "): controlla che non sia un pezzo vero.")
	}
	switch {
	case grafoNonCompleto && motivoGrafo != "":
		b.WriteString(" Il grafo del file non è completo (" + motivoGrafo + "): che il nodo non abbia figli potrebbe non essere un'informazione.")
	case grafoNonCompleto:
		b.WriteString(" Il grafo del file non è completo: che il nodo non abbia figli potrebbe non essere un'informazione.")
	}
	return b.String()
}

// elencoParole: «A», «A» e «B», «A», «B» e «C».
func elencoParole(parole []string) string {
	q := make([]string, len(parole))
	for i, p := range parole {
		q[i] = "«" + p + "»"
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " e " + q[len(q)-1]
}
