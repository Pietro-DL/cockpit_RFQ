package ancoraggio

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// L1 — la regola v1 del segnale LS-02 dentro il pacchetto (elementi_cad.go; Q-01, la parte dell'espressione): dove
// stanno le parole (paroleDelGrezzo), la coerenza fra l'espressione che decide e quella che localizza, e l'equivalenza con
// l'espressione Python della misura su tutti i testi composti da un alfabeto di pezzi ASCII (più «é»), confrontata con
// una lettura indipendente della semantica Python (lookbehind e lookahead su [a-z] con re.I). I testi sono sintetici.

// TestParoleDelGrezzo: le parole com'erano nel testo e i loro intervalli in byte, anche quando il confine destro di una
// parola è il confine sinistro della successiva, e dopo un carattere non ASCII.
func TestParoleDelGrezzo(t *testing.T) {
	for _, c := range []struct {
		testo      string
		parole     []string
		intervalli []evidenze.Intervallo
	}{
		{"ACME_111_2.000_Saldatura d'angolo 1_oa_0", []string{"Saldatura"}, []evidenze.Intervallo{{Inizio: 15, Fine: 24}}},
		{"FILLET_WELD_ID_3", []string{"FILLET", "WELD_ID"}, []evidenze.Intervallo{{Inizio: 0, Fine: 6}, {Inizio: 7, Fine: 14}}},
		{"bead_bead", []string{"bead", "bead"}, []evidenze.Intervallo{{Inizio: 0, Fine: 4}, {Inizio: 5, Fine: 9}}},
		{"weld   seam", []string{"weld   seam"}, []evidenze.Intervallo{{Inizio: 0, Fine: 11}}},
		{"x-cordone-y", []string{"cordone"}, []evidenze.Intervallo{{Inizio: 2, Fine: 9}}},
		{"àbead", []string{"bead"}, []evidenze.Intervallo{{Inizio: 2, Fine: 6}}},
		{"ACME-9001.206_1-WELD BOLT M6x1x20", nil, nil},
		{"saldature", nil, nil},
		{"", nil, nil},
	} {
		parole, intervalli := paroleDelGrezzo(c.testo)
		if !reflect.DeepEqual(parole, c.parole) || !reflect.DeepEqual(intervalli, c.intervalli) {
			t.Errorf("%q: parole %q %v, attese %q %v", c.testo, parole, intervalli, c.parole, c.intervalli)
		}
	}
}

// TestSegnaliElementoCADLeTreCondizioni: le tre condizioni della regola, una per volta, con i contro-indizi che non
// cambiano l'esito.
func TestSegnaliElementoCADLeTreCondizioni(t *testing.T) {
	cordone := NodoStruttura{Rif: "nodo:x:#2", Grezzi: []ValoreGrezzo{
		{Testo: "ACME Saldatura 2", Campo: CampoGrezzoID, UnitaID: "u:step:#2:id"},
		{Testo: "ACME Saldatura 2", Campo: CampoGrezzoNome, UnitaID: "u:step:#2:nome"}}}
	pezzo := NodoStruttura{Rif: "nodo:x:#3", Grezzi: []ValoreGrezzo{{Testo: "ACME-P-003", Campo: CampoGrezzoID, UnitaID: "u:step:#3:id"}}}
	letto := cordone
	letto.Letture = []motorea.LetturaCodice{{ID: "l:1", Forma: motorea.LetturaForma{Originale: "7120002"}}, {ID: "l:2", Forma: motorea.LetturaForma{Originale: "7120002"}}}
	for _, c := range []struct {
		nome                          string
		n                             NodoStruttura
		foglia, radice, grafoCompleto bool
		segnali                       int
		contro                        []string
	}{
		{"cordone foglia non radice", cordone, true, false, true, 1, nil},
		{"cordone con figli", cordone, false, false, true, 0, nil},
		{"cordone radice del file", cordone, true, true, true, 0, nil},
		{"pezzo vero", pezzo, true, false, true, 0, nil},
		{"cordone con il grafo non completo", cordone, true, false, false, 1, []string{ControIndizioGrafoNonCompleto}},
		{"cordone con un codice letto e il grafo non completo", letto, true, false, false, 1,
			[]string{ControIndizioLetturaIdentita, ControIndizioGrafoNonCompleto}},
	} {
		s := segnaliElementoCAD(c.n, c.foglia, c.radice, c.grafoCompleto, "2 radici nel file: una distinta ne ha una")
		if len(s) != c.segnali {
			t.Errorf("%s: %d segnali, attesi %d", c.nome, len(s), c.segnali)
			continue
		}
		if len(s) == 0 {
			continue
		}
		if !reflect.DeepEqual(s[0].ControIndizi, c.contro) || len(s[0].Riscontri) != 2 || s[0].Riscontri[0].Campo != CampoGrezzoID ||
			s[0].Riscontri[1].Campo != CampoGrezzoNome {
			t.Errorf("%s: %+v", c.nome, s[0])
		}
	}
	s := segnaliElementoCAD(letto, true, false, true, "")[0]
	if !strings.HasSuffix(s.Spiegazione, " Ha anche un codice letto (7120002): controlla che non sia un pezzo vero.") {
		t.Errorf("le letture con lo stesso letterale si dicono una volta: %q", s.Spiegazione)
	}

	// Le forme della spiegazione dell'addendum (AD-LS02-3) che gli esempi di Q-01 non hanno: due letterali diversi
	// («dei codici letti (X, Y)», nell'ordine delle letture, senza doppioni) e tre parole nello stesso campo
	// («X», «Y» e «Z»).
	dueCodici := cordone
	dueCodici.Letture = []motorea.LetturaCodice{{ID: "l:1", Forma: motorea.LetturaForma{Originale: "7120002"}},
		{ID: "l:2", Forma: motorea.LetturaForma{Originale: "7120003"}}, {ID: "l:3", Forma: motorea.LetturaForma{Originale: "7120002"}}}
	if s := segnaliElementoCAD(dueCodici, true, false, true, ""); len(s) != 1 || !strings.HasSuffix(s[0].Spiegazione,
		"e la decisione è tua. Ha anche dei codici letti (7120002, 7120003): controlla che non sia un pezzo vero.") {
		t.Errorf("due letterali diversi: %+v", s)
	}
	// Solo l'id e il nome sono ingressi della regola (EM §11.1, la tabella degli ingressi): un altro campo grezzo con la
	// parola, se StrutturaDa un giorno lo portasse fra i grezzi (per esempio la descrizione), non dà il segnale.
	altroCampo := NodoStruttura{Rif: "nodo:x:#5", Grezzi: []ValoreGrezzo{{Testo: "ACME-P-005", Campo: CampoGrezzoID, UnitaID: "u:step:#5:id"},
		{Testo: "piastra per saldatura", Campo: "descrizione", UnitaID: "u:step:#5:descrizione"}}}
	if s := segnaliElementoCAD(altroCampo, true, false, true, ""); s != nil {
		t.Errorf("la parola in un campo che non è l'id né il nome: %+v", s)
	}
	trePezzi := NodoStruttura{Rif: "nodo:x:#4", Grezzi: []ValoreGrezzo{{Testo: "FILLET_WELD_ID_BEAD", Campo: CampoGrezzoID, UnitaID: "u:step:#4:id"}}}
	tre := segnaliElementoCAD(trePezzi, true, false, true, "")
	if len(tre) != 1 || !reflect.DeepEqual(tre[0].Riscontri[0].Parole, []string{"FILLET", "WELD_ID", "BEAD"}) ||
		!strings.HasPrefix(tre[0].Spiegazione, "L'id del pezzo nel file contiene «FILLET», «WELD_ID» e «BEAD»; il nodo non ha figli") {
		t.Errorf("tre parole nello stesso campo: %+v", tre)
	}
}

// TestSpiegazioneElementoCADITestiDiP5 (decisione P-5 dell'ondata A di V1; addendum ad A1c §2.2, rev. 3; EM §11.1): le
// due frasi del segnale e del grafo. La frase del segnale è vera anche per un nodo già scartato; quella del grafo riporta
// il motivo della completezza tale e quale (qualunque motivo, anche uno che non nasconde figli), e senza motivo resta
// senza parentesi.
func TestSpiegazioneElementoCADITestiDiP5(t *testing.T) {
	cordone := NodoStruttura{Rif: "nodo:x:#2", Grezzi: []ValoreGrezzo{{Testo: "ACME Saldatura 2", Campo: CampoGrezzoNome, UnitaID: "u:step:#2:nome"}}}
	frase := "Il nome del pezzo nel file contiene «Saldatura»; il nodo non ha figli e non è una radice del file: possibile elemento CAD " +
		"(un cordone di saldatura o simile), non un articolo. È solo un segnale: da solo non toglie il nodo dalla proposta né il suo 2D, " +
		"e la decisione è tua."
	if s := segnaliElementoCAD(cordone, true, false, true, ""); len(s) != 1 || s[0].Spiegazione != frase {
		t.Errorf("il grafo completo: %+v", s)
	}
	for _, motivo := range []string{"2 radici nel file: una distinta ne ha una", "struttura v4: gli scarti non sono numerati, serve la rianalisi v3"} {
		atteso := frase + " Il grafo del file non è completo (" + motivo + "): che il nodo non abbia figli potrebbe non essere un'informazione."
		if s := segnaliElementoCAD(cordone, true, false, false, motivo); len(s) != 1 || s[0].Spiegazione != atteso {
			t.Errorf("il grafo non completo con il motivo %q: %+v", motivo, s)
		}
	}
	senzaMotivo := frase + " Il grafo del file non è completo: che il nodo non abbia figli potrebbe non essere un'informazione."
	if s := segnaliElementoCAD(cordone, true, false, false, ""); len(s) != 1 || s[0].Spiegazione != senzaMotivo {
		t.Errorf("il grafo non completo senza motivo: %+v", s)
	}
	// il motivo da solo non dà il contro-indizio: lo dà il grafo non completo
	if s := segnaliElementoCAD(cordone, true, false, true, "2 radici nel file: una distinta ne ha una"); len(s) != 1 || s[0].Spiegazione != frase ||
		len(s[0].ControIndizi) != 0 {
		t.Errorf("un motivo con il grafo completo: %+v", s)
	}
}

// letteraPython: una lettera per [a-z] con re.I, sui byte: solo le 52 lettere ASCII (i testi qui non hanno le quattro
// lettere non ASCII che Python vi aggiunge).
func letteraPython(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// parolaPython: una lettura indipendente dell'espressione Python della misura, sui testi ASCII: per ogni posizione, in
// ordine, prima le quattro parole, poi weld seguito da separatori e da bead, seam o id, con i due confini a larghezza
// zero; dopo una parola si riparte dalla sua fine, come finditer. Dà le parole trovate.
func parolePython(testo string) []string {
	confineSinistro := func(p int) bool { return p == 0 || !letteraPython(testo[p-1]) }
	confineDestro := func(p int) bool { return p == len(testo) || !letteraPython(testo[p]) }
	uguale := func(p int, w string) bool { return p+len(w) <= len(testo) && strings.EqualFold(testo[p:p+len(w)], w) }
	var out []string
	for p := 0; p < len(testo); {
		fine := -1
		if confineSinistro(p) {
			for _, w := range []string{"saldatura", "cordone", "fillet", "bead"} {
				if uguale(p, w) && confineDestro(p+len(w)) {
					fine = p + len(w)
					break
				}
			}
			if fine < 0 && uguale(p, "weld") {
				q := p + 4
				for q < len(testo) && strings.IndexByte(" _-", testo[q]) >= 0 {
					q++
				}
				for _, w := range []string{"bead", "seam", "id"} {
					if uguale(q, w) && confineDestro(q+len(w)) {
						fine = q + len(w)
						break
					}
				}
			}
		}
		if fine < 0 {
			p++
			continue
		}
		out = append(out, testo[p:fine])
		p = fine
	}
	return out
}

// TestRegolaV1EquivaleAllaRegolaPython (Q-01): su tutti i testi di al più tre pezzi di un alfabeto che mette accanto le
// parole, i loro pezzi, i separatori (anche due di seguito, « _»: fra weld e la parola dopo ce ne possono essere più
// d'uno), una cifra, una lettera e «é», l'espressione Go che decide dà lo stesso esito della lettura indipendente della
// semantica Python, e quella che localizza trova le stesse parole, nello stesso ordine. L'espressione che decide è quella
// del pacchetto (espressioneElementoCADV1), la stessa di segnaliElementoCAD.
func TestRegolaV1EquivaleAllaRegolaPython(t *testing.T) {
	pezzi := []string{"saldatura", "cordone", "fillet", "bead", "weld", "seam", "id", "s", "x", "W", " ", "_", "-", " _", ".", "1", "é", "ing"}
	parola := regexp.MustCompile(`(?i)^(?:saldatura|cordone|fillet|bead|weld[ _-]*(?:bead|seam|id))$`)
	testi := []string{""}
	for _, a := range pezzi {
		testi = append(testi, a)
		for _, b := range pezzi {
			testi = append(testi, a+b)
			for _, c := range pezzi {
				testi = append(testi, a+b+c)
			}
		}
	}
	veri := 0
	for _, testo := range testi {
		python := parolePython(testo)
		decide := espressioneElementoCADV1.MatchString(testo)
		parole, intervalli := paroleDelGrezzo(testo)
		if decide != (len(python) > 0) {
			t.Errorf("%q: Go %v, Python %q", testo, decide, python)
		}
		if decide != (len(parole) > 0) || !reflect.DeepEqual(parole, python) {
			t.Errorf("%q: decide %v, parole %q, Python %q", testo, decide, parole, python)
		}
		for i, p := range parole {
			if testo[intervalli[i].Inizio:intervalli[i].Fine] != p || !parola.MatchString(p) {
				t.Errorf("%q: parola %q all'intervallo %v", testo, p, intervalli[i])
			}
		}
		if decide {
			veri++
		}
	}
	if veri == 0 || veri == len(testi) {
		t.Errorf("%d testi con la parola su %d: la prova non distingue niente", veri, len(testi))
	}
}
