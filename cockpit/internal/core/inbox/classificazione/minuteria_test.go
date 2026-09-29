package classificazione

// L1 — la minuteria dal nome di un pezzo (giro 4, fase 4.4a.1a; domanda 30, seconda risposta; studio
// docs/specs/studio_normati_29-09.md § 6.5). Le trenta righe INVENTATI della sonda dello studio (sonda_normati.py,
// fuori da git), con le trappole scritte apposta e l'esito atteso: dati tutti inventati, forma ACME 7120101…7120130.

import (
	"strings"
	"testing"
)

// Le trenta righe della sonda: il nome e l'esito atteso. Il codice ACME davanti al nome e' quello della riga della
// sonda: non conta, e' la forma «codice_descrizione» dei PRODUCT veri.
var minuteriaInventata = []struct{ codice, nome, atteso string }{
	{"7120101", "PIASTRA CON DADI DIN 928 SALDATI", MinuteriaDaVedere},
	{"7120102", "VITE SPECIALE M10 A DISEGNO 7120102", MinuteriaDaVedere},
	{"7120103", "DADO M8 DIN 934 MODIFICATO", MinuteriaDaVedere},
	{"7120104", "VITE ISO 4762 M8X30 ACCORCIATA A 25", MinuteriaDaVedere},
	{"7120105", "PERNO D20 L80 C45", MinuteriaNo},
	{"7120106", "BUSSOLA D30 L40", MinuteriaNo},
	{"7120107", "DISTANZIALE D20 L15", MinuteriaNo},
	{"7120108", "RONDELLA D.40 SP.4 S235JR", MinuteriaDaVedere},
	{"7120109", "RONDELLA 30X10,5X4", MinuteriaDaVedere},
	{"7120110", "PIN D25 L34", MinuteriaDaVedere},
	{"7120111", "SUPPORTO PER VITE M10", MinuteriaDaVedere},
	{"7120112", "STAFFA PORTA DADO", MinuteriaDaVedere},
	{"7120113", "BOLT PLATE", MinuteriaDaVedere},
	{"7120114", "NUT HOLDER M8", MinuteriaDaVedere},
	{"7120115", "SCREW CONVEYOR D300", MinuteriaDaVedere},
	{"7120116", "LINGUETTA 10X40 SP.3", MinuteriaDaVedere},
	{"7120117", "WELD NUT M8", MinuteriaProbabile},
	{"7120118", "HEX BOLT M10X30 8.8", MinuteriaProbabile},
	{"7120119", "SECHSKANTSCHRAUBE ISO 4017 M8X20 8.8", MinuteriaNormato},
	{"7120120", "ECROU HEXAGONAL M8 ISO 4032", MinuteriaNormato},
	{"7120121", "ZYLINDERSTIFT ISO 2338 6X20", MinuteriaNormato},
	{"7120122", "SPINA ELASTICA 6X30 ISO 8752", MinuteriaNormato},
	{"7120123", "VIS CHC M6X20 ISO 4762 CL 12.9", MinuteriaNormato},
	{"7120124", "SPLIT PIN 4X40", MinuteriaProbabile},
	{"7120125", "DADO AUTOBLOCCANTE M10", MinuteriaProbabile},
	{"7120126", "TOLLERANZE GENERALI ISO 2768-mK", MinuteriaNo},
	{"7120127", "S235JR EN 10025-2", MinuteriaNo},
	{"7120128", "WELD BEAD 3", MinuteriaNo},
	{"7120129", "GRANO M6X10 UNI 5923", MinuteriaNormato},
	{"7120130", "PERNO CON TESTA D10X40 ISO 2341-B", MinuteriaNormato},
}

// Le trenta righe della sonda danno l'esito atteso, con il nome solo e con il codice davanti («7120119_SECHSKANT…»,
// la forma di un PRODUCT vero): il codice in testa non conta. Una proposta c'e' solo per «normato» e «probabile».
func TestMinuteriaLeTrentaRigheDellaSonda(t *testing.T) {
	if len(minuteriaInventata) != 30 {
		t.Fatalf("le righe della sonda sono %d, non 30", len(minuteriaInventata))
	}
	for _, r := range minuteriaInventata {
		for _, nome := range []string{r.nome, r.codice + "_" + r.nome} {
			e := Minuteria(nome)
			if e.Esito != r.atteso {
				t.Errorf("%q: %s (%s), atteso %s", nome, e.Esito, e.Frase(), r.atteso)
			}
			if e.Proposta() != (r.atteso == MinuteriaNormato || r.atteso == MinuteriaProbabile) {
				t.Errorf("%q: proposta %v con l'esito %s", nome, e.Proposta(), e.Esito)
			}
		}
	}
}

// Il motivo in parole: la norma e i segnali, il nome di minuteria e i segnali, i perche' di un «da vedere», la norma
// non di pezzo di un «no».
func TestMinuteriaIlMotivoInParole(t *testing.T) {
	for _, c := range []struct{ nome, frase string }{
		{"VITE TE M8X20 UNI 5739 8.8", "normato: UNI 5739 · M8X20 · 8.8"},
		{"SECHSKANTSCHRAUBE ISO 4017 M8X20 8.8", "normato: ISO 4017 · M8X20 · 8.8"},
		{"VITE SDPR M6x22 / WELD SCREW M6x22", "probabile minuteria: VITE · M6X22 · SDPR"},
		{"WELD NUT M8", "probabile minuteria: NUT · M8 · WELD"},
		{"RONDELLA D.40 SP.4 S235JR", "da vedere: nome «RONDELLA»; segno di pezzo fatto: S235JR"},
		{"SCREW CONVEYOR D300", "da vedere: nome «SCREW»; nessuna misura da minuteria"},
		{"SUPPORTO PER VITE M10", "da vedere: nome «VITE»; nome custom: SUPPORTO"},
		{"TOLLERANZE GENERALI ISO 2768-mK", ""},
	} {
		if got := Minuteria(c.nome).Frase(); got != c.frase {
			t.Errorf("%q: %q, attesa %q", c.nome, got, c.frase)
		}
	}
	if e := Minuteria("TOLLERANZE GENERALI ISO 2768-mK"); len(e.Motivi) != 1 || e.Motivi[0] != "norma non di pezzo: ISO 2768" {
		t.Errorf("il «no» nomina la norma di tolleranze: %+v", e)
	}
}

// Le trappole dello studio (§ 4.3 e § 4.5): un nome custom in testa a una meta' ferma l'altra; i segni di pezzo
// fatto fermano anche una norma; una norma di tolleranze, di materiale o di saldatura non e' un pezzo; «UNI n» vale
// solo con i numeri UNI della lista (la correzione della verifica: UNI 4017 non e' ISO 4017).
func TestMinuteriaLeTrappole(t *testing.T) {
	for _, c := range []struct{ nome, atteso string }{
		{"SUPPORTO PERNO / PIN SUPPORT", MinuteriaDaVedere},
		{"DADO SDPR M6 / NUT PROJ WELD M6", MinuteriaProbabile},
		{"PIASTRA+PERNO+ROSETTA ZINCATA DA MONTARE", MinuteriaDaVedere},
		{"VITE A DIS. 7120102 DA ZINCARE", MinuteriaDaVedere},
		{"ROSETTA SPECIALE LASER", MinuteriaDaVedere},
		{"BLOCCA DADO INFERIORE S355JR SP.6", MinuteriaDaVedere},
		{"LAMIERA S235JR UNI EN 10025-2", MinuteriaNo},
		{"TOLLERANZE UNI EN ISO 13920-BE", MinuteriaNo},
		{"VITE UNI 4017", MinuteriaDaVedere},
		{"VITE UNI 4017 M8X20", MinuteriaProbabile},
		{"VITE UNI EN 24017 M8X20", MinuteriaNormato},
		{"DADO M8 DIN934", MinuteriaNormato},
		{"", MinuteriaNo},
		{"7121007", MinuteriaNo},
		{"7121007 NOT SPECIFIED", MinuteriaNo},
	} {
		if e := Minuteria(c.nome); e.Esito != c.atteso {
			t.Errorf("%q: %s (%s), atteso %s", c.nome, e.Esito, e.Frase(), c.atteso)
		}
	}
}

// Il riconoscitore si applica al NOME di un pezzo, mai al testo intero del disegno (studio § 4.5). Sul cartiglio
// inventato la norma delle tolleranze, quella del materiale e quella della saldatura si leggono tutte, e nessuna e'
// di minuteria: la regola A non scatta mai su un cartiglio. Le parole invece si': la nota che parla di dadi a saldare
// M8 farebbe nascere un dado che nella distinta non c'e'. E' per questo che chi lo usa (l'albero proposto) gli passa
// soltanto il nome del pezzo.
func TestMinuteriaNonSulTestoDelDisegno(t *testing.T) {
	cartiglio := "TOLLERANZE GENERALI UNI EN 22768-m MATERIALE S235JR UNI EN 10025-2 SALDATURE SECONDO UNI EN ISO 13920-BE ACME 7120001"
	pezzo, nonPezzo := norme(normaNome(cartiglio))
	if len(pezzo) != 0 || strings.Join(nonPezzo, ", ") != "UNI EN 22768, UNI EN 10025, UNI EN ISO 13920" {
		t.Errorf("le norme del cartiglio: di pezzo %v, non di pezzo %v", pezzo, nonPezzo)
	}
	if e := Minuteria(cartiglio); e.Esito != MinuteriaNo || e.Frase() != "" || strings.Join(e.Motivi, "") != "norma non di pezzo: UNI EN 22768" {
		t.Errorf("il cartiglio: %+v", e)
	}
	nota := cartiglio + " NOTE: I FORI M8 SI POSSONO FARE CON DADI A SALDARE M8 / HOLES M8 WITH WELD NUT M8"
	if e := Minuteria(nota); !e.Proposta() {
		t.Errorf("la trappola della nota non scatta piu' (%s): la prova non dice piu' perche' il testo del disegno resta fuori", e.Frase())
	}
}

// La normalizzazione del nome: maiuscolo, il diametro, la X della misura, il trattino basso, la norma attaccata, gli
// spazi Unicode e le forme a larghezza piena.
func TestMinuteriaNormaNome(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{"Dado  UNI5739  m8×20", "DADO UNI 5739 M8X20"},
		{"vite_tcei_ø6*20", "VITE TCEI D6X20"},
		{"ROND UNIEN 24017", "ROND UNI EN 24017"},
		{"VITE M8 X20", "VITE M8 X20"},
		{"ＶＩＴＥ Ｍ８", "VITE M8"},
		{"Schweißmutter", "SCHWEISSMUTTER"},
	} {
		if got := normaNome(c.in); got != c.out {
			t.Errorf("normaNome(%q) = %q, atteso %q", c.in, got, c.out)
		}
	}
}
