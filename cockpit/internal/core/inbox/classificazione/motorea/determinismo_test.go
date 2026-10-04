package motorea

import (
	"bytes"
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — il determinismo del riconoscimento (A1a-DET; piano A, par.3.4, 4.7.2; v3 §4.2): con famiglie, forme,
// esempi, selettori e letterali in un altro ordine, due compilazioni danno lo stesso snapshot e, su ogni
// testo, le stesse letture nello stesso ordine e con gli stessi byte canonici; due esecuzioni sullo stesso
// motore anche. Fissa anche la versione dell'algoritmo (par.4.6.1).
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
