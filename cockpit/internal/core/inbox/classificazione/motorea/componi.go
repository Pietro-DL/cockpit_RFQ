package motorea

import (
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// VersioneComposizione: l'algoritmo del compositore (A1c, B3; T-B0-17). È una versione propria, accanto a
// VersioneAlgoritmo, che non cambia: il compositore non tocca il riconoscimento né l'interpretazione, e B6 la
// mette nell'impronta dell'esito. Cambiare le regole di questo file (quali parti si scrivono, quando una parte
// è determinata, l'ordine dei motivi) vuol dire cambiarla, con un commit che lo dichiara.
const VersioneComposizione = "composizione-1"

// CodiceComposto: il codice proposto nella forma documentale della famiglia (R63 B), composto con le parti della
// forma ATTIVA su cartiglio.codice della stessa famiglia. Testo "" vuol dire «non determinato»: il motivo lo dice,
// e una stringa non si inventa mai (B3, A3.3; R87: niente stringa canonica senza revisione).
//
// È un valore calcolato, mai una decisione: non cambia la lettura da cui viene (LetturaForma e LetturaCodice
// restano quelle di A1a e A1b, D2), e la catena del codice di A1c lo tiene accanto al grezzo e alla lettura, mai
// al loro posto (6.0.6; I-6).
type CodiceComposto struct {
	Testo    string `json:"testo"`            // per esempio «9123456A2» dalla lettura di «9123456A_2» (fixture ACME)
	Famiglia string `json:"famiglia"`         // la famiglia della lettura
	Forma    string `json:"forma,omitempty"`  // l'ID della forma su cartiglio.codice usata; "" se nessuna
	Motivo   string `json:"motivo,omitempty"` // MotivoComposizione*; "" quando Testo c'è
}

// I motivi di un CodiceComposto senza testo (contratto A1c §2.1; T-B0-37). Non sono codici di diagnostica: il
// motivo sta nel CodiceComposto, e motorea non ha codici nuovi per R63 (T-15).
//
// Quando ne valgono più d'uno, il motivo è il primo di quest'ordine fisso: prima la grammatica (nessuna forma,
// più forme), poi la lettura nel suo insieme (non completa, revisione ambigua), poi la composizione parte per
// parte (revisione non determinata, parte non determinata) e infine ciò che della lettura resta senza posto
// (qualificatore non trasferibile). La revisione non determinata viene prima delle altre parti perché è il segno
// dell'identità parziale di R87, che la catena del codice deve poter leggere.
const (
	// La famiglia non ha una forma attiva su cartiglio.codice: una forma riservata, o che non entra nel motore,
	// non conta (solo forme compilate, T-B0-17).
	MotivoComposizioneFormaAssente = "forma_cartiglio_assente"
	// Ne ha più d'una: il compositore non sceglie (R63 B).
	MotivoComposizioneFormeMultiple = "forme_cartiglio_multiple"
	// Una parte della forma non viene dalla lettura, o il suo testo non è uno solo, o è un'etichetta o una
	// decorazione, che stanno fuori dall'identità e non entrano nella stringa proposta (T-B3-02).
	MotivoComposizioneParteNonDeterminata = "parte_non_determinata"
	// Un qualificatore letto (affisso, marcatore, token o revisione della lettura) non ha posto nella forma del
	// cartiglio: toglierlo cambierebbe il codice.
	MotivoComposizioneQualificatoreNonTrasferibile = "qualificatore_non_trasferibile"
	// La forma ha la revisione e la lettura non ce l'ha, come i nodi STEP che leggono base e marcatore senza
	// revisione (T-B0-37, R87). È distinto da revisione_ambigua (LD-14).
	MotivoComposizioneRevisioneNonDeterminata = "revisione_non_determinata"
	// La revisione c'è ma non è «letta»: un token sospeso, non_interpretabile (D5).
	MotivoComposizioneRevisioneAmbigua = "revisione_ambigua"
	// La lettura è parziale o discordante (A-C07, D2), o non è della famiglia che dichiara.
	MotivoComposizioneLetturaNonCompleta = "lettura_non_completa"
)

// selettoreCartiglioCodice: il campo del codice nel cartiglio, dove sta la forma documentale di una famiglia (R63
// B). È la coppia di evidenze.LeggiSelettore("cartiglio.codice"), scritta qui perché è una chiave dei piani.
var selettoreCartiglioCodice = evidenze.Selettore{
	Contesto: evidenze.ContestoCartiglio,
	Campo:    evidenze.CampoFonte{Variante: evidenze.VarianteCartiglio, Valore: "codice"},
}

// ComponiCodiceDocumentale: dal dato grezzo letto (una LetturaForma, per esempio di un nodo STEP o del nome del
// file: base 9123456, marcatore A, revisione 2 da «9123456A_2») alla stringa nella forma documentale della
// stessa famiglia, cioè la forma attiva su cartiglio.codice (R63 B): «9123456A2». È un metodo nuovo: non
// cambia nessuna firma di A1a e A1b (D2), e la lettura che riceve resta com'è.
//
// Usa solo le parti della forma compilata e quelle della lettura: nessun lessico, nessuna regola per cliente
// nel codice (par.12). Le regole, parte per parte della forma del cartiglio:
//   - la base (e la sua ripetizione) è la base normalizzata della lettura, che deve essere completa;
//   - marcatore, token e affissi vengono dalla lettura: il valore letto si scrive se la forma lo ammette (per
//     gli affissi, la stessa regola). Una parte facoltativa senza valore resta fuori solo se la forma che ha
//     prodotto la lettura aveva quel posto, cioè se l'assenza è letta; altrimenti non è determinata;
//   - la revisione viene dalla lettura, della stessa regola, con il separatore della regola se è uno solo. Una
//     lettura senza revisione non dà stringa se la forma chiede o ammette la revisione (R87: senza revisione
//     nessuna stringa canonica; T-B3-01);
//   - i separatori fanno parte del codice e non vengono dalla lettura: si scrivono solo se il loro testo è uno
//     solo (un letterale);
//   - etichette e decorazioni stanno fuori dall'identità, e la stringa proposta servirà alla rinomina: se la
//     forma ne ha una, niente stringa (T-B3-02);
//   - della lettura restano fuori, perché la grammatica li dichiara fuori dal codice, solo l'etichetta, le
//     decorazioni e le ripetizioni concordanti; ogni altra parte letta (affisso, marcatore, token, revisione)
//     deve avere il suo posto nella forma.
//
// Alla fine la stringa si rilegge con Riconosci su cartiglio.codice: deve darla per intero la stessa forma, con
// la stessa base, lo stesso marcatore, lo stesso token, la stessa revisione e gli stessi affissi. Se no, la
// stringa non è determinata in modo univoco e non si dà.
//
// È deterministica: niente orologio, file, rete, mappe in ordine; lo stesso motore e la stessa lettura danno lo
// stesso CodiceComposto. Un motore nullo non ha forme.
func (m *Motore) ComponiCodiceDocumentale(l LetturaForma) CodiceComposto {
	c := CodiceComposto{Famiglia: l.Famiglia}
	forme := m.formeDocumentali(l.Famiglia)
	switch {
	case len(forme) == 0:
		c.Motivo = MotivoComposizioneFormaAssente
		return c
	case len(forme) > 1:
		c.Motivo = MotivoComposizioneFormeMultiple
		return c
	}
	p := forme[0]
	c.Forma = p.forma
	if motivo := motivoDellaLettura(p, l); motivo != "" {
		c.Motivo = motivo
		return c
	}
	testo, motivo := componi(p, m.formaDellaLettura(l), l)
	if motivo == "" && !m.rilegge(p, testo, l) {
		motivo = MotivoComposizioneParteNonDeterminata
	}
	if motivo != "" {
		c.Motivo = motivo
		return c
	}
	c.Testo = testo
	return c
}

// formeDocumentali: le forme della famiglia compilate nel motore su cartiglio.codice, in ordine di forma. Sono
// le forme attive con il selettore attivo e senza parti riservate: le altre non hanno piani (motore.go, abbassa).
func (m *Motore) formeDocumentali(famiglia string) []*pianoForma {
	if m == nil {
		return nil
	}
	var out []*pianoForma
	for _, p := range m.piani[selettoreCartiglioCodice] {
		if p.famiglia == famiglia {
			out = append(out, p)
		}
	}
	return out
}

// formaDellaLettura: il piano che ha prodotto la lettura, per selettore, famiglia e forma. Nil se il motore non
// lo ha (una lettura costruita a mano, o di un altro motore): allora nessuna assenza è letta.
func (m *Motore) formaDellaLettura(l LetturaForma) *pianoForma {
	for _, p := range m.piani[l.Selettore] {
		if p.famiglia == l.Famiglia && p.forma == l.Forma {
			return p
		}
	}
	return nil
}

// motivoDellaLettura: la lettura nel suo insieme. Lettura non completa se è parziale o discordante, se la base
// non ha tutti i segmenti, se una ripetizione non concorda, se lo stato non è uno di quelli che il motore dà o
// se il namespace non è quello della famiglia; revisione ambigua se la revisione c'è ma non è letta (lo stato
// da_verificare vuol dire un token sospeso: la base è una candidata, D5).
func motivoDellaLettura(p *pianoForma, l LetturaForma) string {
	completa := l.Namespace == p.namespace && l.Base.Completa && len(l.Base.Mancanti) == 0 &&
		l.Base.Normalizzata != "" && len(l.Base.Segmenti) > 0
	for _, r := range l.Ripetizioni {
		if !r.Concorda {
			completa = false
		}
	}
	if l.Stato != StatoCompleta && l.Stato != StatoDaVerificare {
		completa = false
	}
	if !completa {
		return MotivoComposizioneLetturaNonCompleta
	}
	if l.Stato == StatoDaVerificare || (l.Revisione != nil &&
		(l.Revisione.Stato != StatoRevisioneLetta || l.Revisione.Normalizzata == "")) {
		return MotivoComposizioneRevisioneAmbigua
	}
	return ""
}

// componi scrive la stringa parte per parte della forma del cartiglio p (le regole sono nel commento di
// ComponiCodiceDocumentale). fonte è la forma che ha prodotto la lettura, o nil. Con una parte non determinata
// dà il motivo, nell'ordine fisso dei motivi, e nessuna stringa.
func componi(p *pianoForma, fonte *pianoForma, l LetturaForma) (string, string) {
	var b strings.Builder
	revisioneMancante := false
	parteMancante := !p.completa // una proiezione del cartiglio non è il codice: vincoli, non un articolo (A-C07)
	postoMarcatore, postoToken, postoRevisione := false, false, false
	affissiUsati := make([]bool, len(l.Affissi))

	for _, pt := range p.scrittura {
		switch pt.tipo {
		case grammatica.TipoParteBase, grammatica.TipoParteRipetizioneBase:
			b.WriteString(l.Base.Normalizzata)
		case grammatica.TipoParteSeparatore:
			// Fa parte del codice: si scrive se il testo è uno solo, mai preso dalla lettura.
			if pt.facoltativa || len(pt.letterali) != 1 {
				parteMancante = true
				continue
			}
			b.WriteString(pt.letterali[0])
		case grammatica.TipoParteEtichetta, grammatica.TipoParteDecorazione:
			// Fuori dall'identità: non entra nella stringa proposta, che servirà alla rinomina, e una forma che la
			// chiede non dà una stringa senza (T-B3-02). Facoltativa resta una scelta che il compositore non fa.
			parteMancante = true
		case grammatica.TipoParteMarcatore:
			postoMarcatore = true
			if !scriviLetta(&b, pt, l.Marcatore, fonte.haParte(pt.tipo, "")) {
				parteMancante = true
			}
		case grammatica.TipoParteToken:
			postoToken = true
			if !scriviLetta(&b, pt, l.Token, fonte.haParte(pt.tipo, "")) {
				parteMancante = true
			}
		case grammatica.TipoParteAffisso:
			// Gli affissi si abbinano per regola, nell'ordine delle parti: la stessa regola vuol dire gli stessi
			// letterali e lo stesso significato (parte 1 §5.3).
			i := affissoLibero(l.Affissi, affissiUsati, pt.rif)
			switch {
			case i >= 0:
				affissiUsati[i] = true
				b.WriteString(l.Affissi[i].Originale)
			case pt.facoltativa && fonte.haParte(pt.tipo, pt.rif):
				// l'assenza è letta: la forma della lettura aveva quel posto e il testo non lo riempie
			default:
				parteMancante = true
			}
		case grammatica.TipoParteRevisione:
			postoRevisione = true
			switch {
			case l.Revisione == nil:
				revisioneMancante = true
			case l.Revisione.Regola != pt.rif || len(pt.separatori) > 1:
				// Una revisione di un'altra regola non si trasferisce: nessuna equivalenza è dichiarata fra le due.
				// Con più separatori il testo non è uno solo, anche se la lettura ne ha usato uno.
				parteMancante = true
			default:
				if len(pt.separatori) == 1 {
					b.WriteString(pt.separatori[0])
				}
				b.WriteString(l.Revisione.Normalizzata) // la stringa com'è, zeri compresi (D4)
			}
		default:
			parteMancante = true // una parte che il compositore non sa scrivere non si inventa
		}
	}

	senzaPosto := (l.Marcatore != nil && !postoMarcatore) || (l.Token != nil && !postoToken) ||
		(l.Revisione != nil && !postoRevisione)
	for _, usato := range affissiUsati {
		if !usato {
			senzaPosto = true
		}
	}
	switch {
	case revisioneMancante:
		return "", MotivoComposizioneRevisioneNonDeterminata
	case parteMancante:
		return "", MotivoComposizioneParteNonDeterminata
	case senzaPosto:
		return "", MotivoComposizioneQualificatoreNonTrasferibile
	}
	return b.String(), ""
}

// scriviLetta: un marcatore o un token, dalla lettura. Il valore letto si scrive se è fra i letterali della parte
// (esatti: le maiuscole della base non valgono per loro). Senza valore, una parte facoltativa resta fuori solo
// se l'assenza è letta; altrimenti la parte non è determinata. L'assenza si giudica dal tipo della parte nella
// forma della lettura (haParte), non dai suoi letterali.
func scriviLetta(b *strings.Builder, pt parteScritta, letta *ParteLetta, assenzaLetta bool) bool {
	switch {
	case letta == nil:
		return pt.facoltativa && assenzaLetta
	case !contiene(pt.letterali, letta.Valore):
		return false
	}
	b.WriteString(letta.Valore)
	return true
}

// affissoLibero: il primo affisso letto della regola data non ancora usato; -1 se non c'è.
func affissoLibero(affissi []AffissoLetto, usati []bool, regola string) int {
	for i, a := range affissi {
		if !usati[i] && a.Regola == regola {
			return i
		}
	}
	return -1
}

// rilegge: la lettura che il motore dà della stringa composta, con Riconosci su cartiglio.codice, è quella di
// partenza: la forma usata la legge per intero e completa, con la stessa base negli stessi segmenti, lo stesso
// marcatore, lo stesso token, la stessa revisione e gli stessi affissi; non un'altra divisione dei segmenti, non
// una revisione presa per token.
func (m *Motore) rilegge(p *pianoForma, testo string, l LetturaForma) bool {
	letture, _ := m.Riconosci(selettoreCartiglioCodice, testo)
	var r *LetturaForma
	for i := range letture {
		x := &letture[i]
		if x.Famiglia != p.famiglia || x.Forma != p.forma || x.Intervallo.Inizio != 0 || x.Intervallo.Fine != len(testo) {
			continue
		}
		if r != nil {
			return false
		}
		r = x
	}
	return r != nil && r.Stato == StatoCompleta && stessaBase(r.Base, l.Base) &&
		stessaParte(r.Marcatore, l.Marcatore) && stessaParte(r.Token, l.Token) &&
		stessaRevisione(r.Revisione, l.Revisione) && stessiAffissi(r.Affissi, l.Affissi)
}

// stessaBase: la stessa base normalizzata, divisa negli stessi segmenti.
func stessaBase(a, b BaseLetta) bool {
	if a.Normalizzata != b.Normalizzata || len(a.Segmenti) != len(b.Segmenti) {
		return false
	}
	for i := range a.Segmenti {
		if a.Segmenti[i].Nome != b.Segmenti[i].Nome || a.Segmenti[i].Normalizzato != b.Segmenti[i].Normalizzato {
			return false
		}
	}
	return true
}

func stessaParte(a, b *ParteLetta) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Valore == b.Valore
}

func stessaRevisione(a, b *RevisioneLetta) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Regola == b.Regola && a.Stato == b.Stato && a.Normalizzata == b.Normalizzata
}

// stessiAffissi: gli stessi affissi, per regola e testo, in qualunque ordine (la forma del cartiglio può
// metterli in un altro ordine delle parti).
func stessiAffissi(a, b []AffissoLetto) bool {
	if len(a) != len(b) {
		return false
	}
	chiavi := func(x []AffissoLetto) []string {
		out := make([]string, len(x))
		for i, af := range x {
			out[i] = af.Regola + "\x00" + af.Originale
		}
		sort.Strings(out)
		return out
	}
	ka, kb := chiavi(a), chiavi(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

// ---- la forma come la legge il compositore ----

// parteScritta: una parte di una forma compilata come serve al compositore. È privata e minima come il piano
// (par.12), non una copia di grammatica.Parte: il tipo, se è facoltativa, la regola o i letterali che dicono che
// cosa può stare al suo posto. Di etichette e decorazioni basta il tipo: non si scrivono mai (T-B3-02). La
// calcola compilaForma una volta, dalla stessa grammatica normalizzata dei piani: dopo CompilaVerificato non
// dipende dalle slice del chiamante (E4).
type parteScritta struct {
	tipo        string
	facoltativa bool
	rif         string   // affisso e revisione: la regola, che la lettura deve avere uguale
	letterali   []string // separatore, marcatore, token: quelli della parte
	separatori  []string // revisione: i separatori della regola
}

// scritturaDi: le parti della forma per il compositore, nell'ordine della forma. Le regole citate si risolvono
// come in compilaForma, che arriva qui solo se le ha risolte tutte; una che non si risolvesse lascerebbe una
// parte senza tipo, che il compositore non scrive.
func scritturaDi(r regoleFamiglia, fo grammatica.FormaCodice) []parteScritta {
	out := make([]parteScritta, 0, len(fo.Parti))
	for _, pt := range fo.Parti {
		ps := parteScritta{tipo: pt.Tipo, facoltativa: pt.Min == 0, rif: pt.Rif}
		switch pt.Tipo {
		case grammatica.TipoParteSeparatore, grammatica.TipoParteMarcatore, grammatica.TipoParteToken:
			ps.letterali = copiaStringhe(pt.Letterali)
		case grammatica.TipoParteRevisione:
			if rv := r.revisioni[pt.Rif]; rv != nil {
				ps.separatori = copiaStringhe(rv.separatori)
			} else {
				ps.tipo = ""
			}
		case grammatica.TipoParteAffisso:
			if r.affisso(pt.Rif) == nil {
				ps.tipo = ""
			}
		}
		out = append(out, ps)
	}
	return out
}

// haParte: la forma ha una parte di quel tipo (per un affisso, di quella regola). Una forma nulla non ha parti.
func (p *pianoForma) haParte(tipo, rif string) bool {
	if p == nil {
		return false
	}
	for _, pt := range p.scrittura {
		if pt.tipo == tipo && (tipo != grammatica.TipoParteAffisso || pt.rif == rif) {
			return true
		}
	}
	return false
}
