package motorea

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — il determinismo del riconoscimento (A1a-DET; piano A, par.3.4, 4.7.2; v3 §4.2): con famiglie, forme,
// esempi, selettori e letterali in un altro ordine, due compilazioni danno lo stesso snapshot e, su ogni
// testo, le stesse letture nello stesso ordine e con gli stessi byte canonici; due esecuzioni sullo stesso
// motore anche. Fissa anche la versione dell'algoritmo (par.4.6.1). Da A1b.10 anche quello di Interpreta (A1b-21):
// grammatica e documento in un altro ordine danno gli stessi byte canonici dell'interpretazione.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono quelle ACME di sintetiche_test.go.

// rovescia: gli stessi elementi in ordine inverso.
func rovescia[T any](s []T) []T {
	out := make([]T, len(s))
	for i, x := range s {
		out[len(s)-1-i] = x
	}
	return out
}

// permutata: la stessa grammatica con ogni elenco senza significato d'ordine rovesciato. Le parti e i
// segmenti restano nel loro ordine, che conta.
func permutata(g grammatica.Grammatica) grammatica.Grammatica {
	p := g
	p.Famiglie = nil
	for _, f := range rovescia(g.Famiglie) {
		f.Ruoli = rovescia(f.Ruoli)
		var forme []grammatica.FormaCodice
		for _, fo := range rovescia(f.Forme) {
			fo.Selettori = rovescia(fo.Selettori)
			var parti []grammatica.Parte
			for _, pt := range fo.Parti {
				pt.Letterali = rovescia(pt.Letterali)
				parti = append(parti, pt)
			}
			fo.Parti = parti
			forme = append(forme, fo)
		}
		f.Forme = forme
		var decorazioni []grammatica.Decorazione
		for _, d := range rovescia(f.Decorazioni) {
			d.Letterali = rovescia(d.Letterali)
			d.Selettori = rovescia(d.Selettori)
			decorazioni = append(decorazioni, d)
		}
		f.Decorazioni = decorazioni
		var affissi []grammatica.Affisso
		for _, a := range rovescia(f.Affissi) {
			a.Riconoscimento = rovescia(a.Riconoscimento)
			a.Attribuzione = rovescia(a.Attribuzione)
			affissi = append(affissi, a)
		}
		f.Affissi = affissi
		var revisioni []grammatica.RegolaRevisione
		for _, r := range rovescia(f.Revisioni) {
			r.Selettori = rovescia(r.Selettori)
			revisioni = append(revisioni, r)
		}
		f.Revisioni = revisioni
		f.Esempi = rovescia(f.Esempi)
		p.Famiglie = append(p.Famiglie, f)
	}
	return p
}

// ingressiDET: testi su più selettori, con codici di più famiglie e di più forme nello stesso testo.
var ingressiDET = []struct{ sel, testo string }{
	{"corpo", "P7120100, 9.123.4567.3/01 e PN 7654321; 12.3456.7890GT S9.123.4567 9.123.4567.3/xx è P7120101"},
	{"oggetto", "RFQ 9.123.4567.3_30 PN 1234567 12.3456.7890"},
	{"storia", "> P7120100\n> P7120102"},
	{"nome_file", "ACME-030P7120100 00 IN_WORK.stp"},
	{"nome_file", "9123456_9123457.zip"},
	{"nome_file", "97123456#1#R98123456#.pdf"},
	{"nome_file", "T+300.012345.010  00.zip"},
	{"nome_file", "912345X_1.stp"},
	{"cartiglio.codice", "9123456A2"},
	{"nodo_step.id", "912345X1"},
	{"radice_step.id", "9123456A_PRT"},
}

func TestDeterminismoConFamiglieEFormePermutate(t *testing.T) {
	g := grammaticaACME(tutteLeFamiglie()...)
	m1, d1 := compilaBene(t, g)
	m2, d2 := compilaBene(t, permutata(g))

	if m1.Snapshot().Hash != m2.Snapshot().Hash || !bytes.Equal(m1.Snapshot().Canonico, m2.Snapshot().Canonico) {
		t.Errorf("snapshot diversi: %s, %s", m1.Snapshot().Hash, m2.Snapshot().Hash)
	}
	if !reflect.DeepEqual(d1, d2) {
		t.Errorf("diagnostiche della compilazione diverse:\n%s\n---\n%s", elenco(d1), elenco(d2))
	}

	// Lo snapshot non normalizzato, dato direttamente: CompilaVerificato lo rende canonico da sé.
	m3, d3, err := CompilaVerificato(grammatica.SnapshotRegole{ClienteID: clienteACME, Grammatica: permutata(g)}, limitiACME())
	if err != nil {
		t.Fatalf("snapshot non normalizzato: %v\n%s", err, elenco(d3))
	}

	for _, in := range ingressiDET {
		l1, x1 := riconosci(t, m1, in.sel, in.testo)
		l2, x2 := riconosci(t, m2, in.sel, in.testo)
		l3, _ := riconosci(t, m3, in.sel, in.testo)
		ancora, _ := riconosci(t, m1, in.sel, in.testo)
		if len(l1) == 0 {
			t.Errorf("%s %q: nessuna lettura, l'ingresso non prova niente", in.sel, in.testo)
		}
		b1 := canonico(t, l1)
		for nome, altre := range map[string][]LetturaForma{"permutata": l2, "non normalizzata": l3, "seconda esecuzione": ancora} {
			if !reflect.DeepEqual(l1, altre) {
				t.Errorf("%s %q, %s: letture diverse\n%s\n%s", in.sel, in.testo, nome, riassunto(l1), riassunto(altre))
			}
			if !bytes.Equal(b1, canonico(t, altre)) {
				t.Errorf("%s %q, %s: byte canonici diversi", in.sel, in.testo, nome)
			}
		}
		if !reflect.DeepEqual(x1, x2) {
			t.Errorf("%s %q: diagnostiche diverse\n%s\n---\n%s", in.sel, in.testo, elenco(x1), elenco(x2))
		}
	}
}

// TestDeterminismoMoltiRiconoscimenti: cento esecuzioni sullo stesso testo danno gli stessi byte canonici:
// nessun ordine dipende da una mappa.
func TestDeterminismoMoltiRiconoscimenti(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	x := selettore(t, ingressiDET[0].sel)
	l, d := m.Riconosci(x, ingressiDET[0].testo)
	primo := canonico(t, l)
	primeDiag := canonico(t, d)
	for i := 0; i < 100; i++ {
		l, d := m.Riconosci(x, ingressiDET[0].testo)
		if !bytes.Equal(primo, canonico(t, l)) || !bytes.Equal(primeDiag, canonico(t, d)) {
			t.Fatalf("esecuzione %d diversa dalla prima", i)
		}
	}
}

// TestDeterminismoVersioneAlgoritmo fissa la versione dell'algoritmo di riconoscimento (par.4.6.1).
// Cambiarla vuol dire riscrivere questa prova con il titolo «Riscritta per …».
func TestDeterminismoVersioneAlgoritmo(t *testing.T) {
	if VersioneAlgoritmo != "motorea-1" {
		t.Errorf("VersioneAlgoritmo = %q, attesa motorea-1", VersioneAlgoritmo)
	}
}

// canonico: i byte del JSON canonico; le letture devono avere una forma canonica (A1a-DET).
func canonico(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatalf("forma canonica: %v", err)
	}
	return b
}

// TestInterpretaEDeterministica (A1b-21, MOTORE-SENZA-LLM nella parte in memoria; 5.4.6 punto 17; par.3.4):
// famiglie, forme, regole della quantità, testi, segmenti, entità e unità in un altro ordine danno lo stesso
// canonico dell'interpretazione, e cento esecuzioni danno gli stessi byte. Il documento ha di tutto: celle esatte e
// per righe, revisione in campo separato, formazione, quantità, alternative, uso valutato.
func TestInterpretaEDeterministica(t *testing.T) {
	g := grammaticaACME(famCodice(), famPrefisso(), famMarcatoreX())
	g.Quantita = []grammatica.QuantitaTabellare{
		{ID: "q-acme", Intestazioni: []string{"Q.TA", "PEZZI"}, Regola: grammatica.RegolaQuantitaPrimaRigaSopra, Selettori: sel("corpo", "storia"), Stato: grammatica.StatoAttiva},
		{ID: "q-acme-2", Intestazioni: []string{"N."}, Regola: grammatica.RegolaQuantitaPrimaRigaSopra, Selettori: sel("storia"), Stato: grammatica.StatoRiservata},
	}
	p := permutata(g)
	p.Quantita = rovescia(g.Quantita)
	p.Quantita[1].Intestazioni = rovescia(p.Quantita[1].Intestazioni)
	p.Quantita[1].Selettori = rovescia(p.Quantita[1].Selettori)
	m1, _ := compilaBene(t, g)
	m2, _ := compilaBene(t, p)

	corpo := "Elenco P7120100:\nCodice\tQ.TA\nACME1111\t5\nACME2222\t3\n> P7120101"
	storia := strings.Index(corpo, "> P7120101")
	doc := docDSint(
		[]evidenze.TestoOriginale{testoOrig(oggettoDSnt, "I: ACME1111"), testoOrig(corpoDSint, corpo)},
		[]evidenze.Segmento{
			segDSint("s:corrente", evidenze.SegmentoCorrente, "m0", corpoDSint, 0, storia),
			segDSint("s:storia:1", evidenze.SegmentoCitazione, "m1", corpoDSint, storia, len(corpo)),
		},
		[]evidenze.EntitaLocale{
			entRiga("e:tab:1:r1", "s:corrente", 1, 1, [2]int{1, 2}),
			entRiga("e:tab:1:r2", "s:corrente", 1, 2, [2]int{2, 3}),
			entRiga("e:tab:1:r3", "s:corrente", 1, 3, [2]int{3, 4}),
			entDisegno("e:pdf:p1"), entNodo("e:step:#10", "#10"), entNodo("e:step:#20", "#20"),
		},
		[]evidenze.UnitaEvidenza{
			uTesto(t, "u:oggetto", "s:corrente", "oggetto", oggettoDSnt, "I: ACME1111", 0),
			uTesto(t, "u:corpo:s:corrente", "s:corrente", "corpo", corpoDSint, corpo[:storia], 0),
			uTesto(t, "u:storia:s:storia:1", "s:storia:1", "storia", corpoDSint, corpo[storia:], storia),
			uCella(t, "u:tab:1:r1:c1", "e:tab:1:r1", "corpo", "Codice", 1, 1, 1, [2]int{1, 2}, nil),
			uCella(t, "u:tab:1:r1:c2", "e:tab:1:r1", "corpo", "Q.TA", 1, 1, 2, [2]int{1, 2}, nil),
			uCella(t, "u:tab:1:r2:c1", "e:tab:1:r2", "corpo", "ACME1111", 1, 2, 1, [2]int{2, 3}, nil),
			uCella(t, "u:tab:1:r2:c2", "e:tab:1:r2", "corpo", "5", 1, 2, 2, [2]int{2, 3}, nil),
			uCella(t, "u:tab:1:r3:c1", "e:tab:1:r3", "corpo", "ACME2222", 1, 3, 1, [2]int{3, 4}, nil),
			uCella(t, "u:tab:1:r3:c2", "e:tab:1:r3", "corpo", "3", 1, 3, 2, [2]int{3, 4}, nil),
			uCampo(t, "u:pdf:cartiglio:codice:1", "e:pdf:p1", "cartiglio.codice", "ACME1111"),
			uCampo(t, "u:pdf:cartiglio:revisione:1", "e:pdf:p1", "cartiglio.revisione", "04"),
			uStep(t, "u:step:#10:id", "e:step:#10", "#10", "radice_step.id", "id", "912345X1_PRT"),
			uStep(t, "u:step:#10:revisione", "e:step:#10", "#10", "radice_step.revisione", "formazione", "1"),
			uStep(t, "u:step:#20:id", "e:step:#20", "#20", "nodo_step.id", "id", "ACME3333"),
		},
		capacita("testo", "disponibile"), capacita("firma", "non_disponibile"))
	doc.Legami = []evidenze.LegameFonte{
		{ID: "l:a", Tipo: "cella_di_riga", Da: "u:tab:1:r2:c1", A: "e:tab:1:r2"},
		{ID: "l:b", Tipo: "padre_figlio_step", Da: "e:step:#10", A: "e:step:#20", Quantita: func() *int { n := 1; return &n }()},
	}
	rovesciato := doc
	rovesciato.Testi = rovescia(doc.Testi)
	rovesciato.Segmenti = rovescia(doc.Segmenti)
	rovesciato.Entita = rovescia(doc.Entita)
	rovesciato.Unita = rovescia(doc.Unita)
	rovesciato.Legami = rovescia(doc.Legami)
	uso := usoValutato(selezione("s:storia:1", UsoPertinente, "scenario"), selezione("s:corrente", UsoPertinente, "operatore"))

	r1 := interpreta(t, m1, doc, uso)
	if len(r1.Letture) == 0 || len(attributiDi(r1, AttributoQuantita)) != 2 || len(attributiDi(r1, AttributoRevisione)) != 1 ||
		len(attributiDi(r1, AttributoFormazione)) != 1 {
		t.Fatalf("il documento non esercita quello che deve: %v %+v", idLetture(r1.Letture), r1.Attributi)
	}
	b1 := canonico(t, r1)
	for nome, r := range map[string]Interpretazione{
		"grammatica permutata": interpreta(t, m2, doc, uso),
		"documento permutato":  interpreta(t, m1, rovesciato, uso),
		"tutto permutato":      interpreta(t, m2, rovesciato, uso),
	} {
		if !bytes.Equal(b1, canonico(t, r)) {
			t.Errorf("%s: canonico diverso", nome)
		}
	}
	for i := 0; i < 100; i++ {
		r, err := m1.Interpreta(doc, uso)
		if err != nil || !bytes.Equal(b1, canonico(t, r)) {
			t.Fatalf("esecuzione %d diversa dalla prima (%v)", i, err)
		}
	}
}
