package motorea

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — la revisione registrata a parte, letta con le regole in campo separato della famiglia (R113 B ratificata;
// emendamento E2 §2.6; A1c, B6b; il precedente è T-B0-17: un metodo solo, con le sue prove): una regola attiva legge la
// colonna, zeri compresi; nessuna regola in campo separato (anche con una regola in linea che leggerebbe il testo) dà
// nessuna_regola; due regole diverse danno ambigua, senza provarle; un token sospeso, una regola riservata, un testo non
// letto per intero o vuoto danno non_interpretabile; la stessa regola su più selettori conta una volta; le regole di
// un'altra famiglia non contano; mai «codice + rev» composti; la versione propria, le firme e il determinismo.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri sono un
// dato dell'azienda. Le grammatiche sono acme-campo-separato, acme-marcatore e acme-punti di sintetiche_test.go, con le
// varianti scritte qui; le prove citano i requisiti (R113, E2 §2.6, D-07, Q1, D5), mai i casi degli attesi.

const famigliaColonna = "acme-campo-separato"

// famColonnaCon: acme-campo-separato con le regole di revisione date al posto della sua; senza l'esempio della
// revisione, che con più regole o con una regola riservata direbbe un'altra cosa (qui si prova il metodo, non gli esempi).
func famColonnaCon(regole ...grammatica.RegolaRevisione) grammatica.FamigliaCodice {
	f := famCampoSeparato()
	f.Revisioni = regole
	var esempi []grammatica.EsempioCodice
	for _, e := range f.Esempi {
		if e.Selettore != "cartiglio.revisione" {
			esempi = append(esempi, e)
		}
	}
	f.Esempi = esempi
	return f
}

// regolaColonna: una regola in campo separato attiva, sui selettori dati, con il pattern del valore dato.
func regolaColonna(id, pattern string, selettori ...string) grammatica.RegolaRevisione {
	return grammatica.RegolaRevisione{
		ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteCampoSeparato,
		Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: pattern, Significato: grammatica.SignificatoNessuno}},
	}
}

// famLettere: un'altra famiglia dello stesso cliente, con la sua regola in campo separato sullo stesso selettore, che
// legge una lettera sola.
func famLettere() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-lettere", Namespace: "acme-lettere",
		Ruoli:     []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:      base(segPattern("codice", "LT[0-9]{4}", "")),
		Forme:     []grammatica.FormaCodice{forma("cartiglio", sel("cartiglio.codice"), pBase())},
		Revisioni: []grammatica.RegolaRevisione{regolaColonna("rev-lettera", "[A-Z]", "cartiglio.revisione")},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-lt", "cartiglio.codice", "LT1234", grammatica.LetturaAttesa{Forma: "cartiglio", Base: "LT1234"}),
		},
	}
}

// controllaRegistrata: il confronto di una RevisioneRegistrata con i campi attesi; la lettura si guarda a parte.
func controllaRegistrata(t *testing.T, nome string, got RevisioneRegistrata, stato, regola, motivo string, conLettura bool) {
	t.Helper()
	if got.Stato != stato || got.Regola != regola || got.Motivo != motivo || (got.Revisione != nil) != conLettura {
		t.Errorf("%s: %+v (lettura %+v), atteso stato %q, regola %q, motivo %q, lettura %v", nome, got, got.Revisione, stato,
			regola, motivo, conLettura)
	}
}

// TestRegistrataUnaRegolaLeggeLaColonna (R113 B; D-07, D4): l'unica regola attiva della famiglia legge il testo intero,
// con gli zeri; l'originale resta com'è, la regola dice famiglia e ID.
func TestRegistrataUnaRegolaLeggeLaColonna(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCampoSeparato()))
	for _, testo := range []string{"01", "00", "10"} {
		got := m.LeggiRevisioneRegistrata(famigliaColonna, testo)
		controllaRegistrata(t, testo, got, StatoRevisioneRegistrataLetta, famigliaColonna+"/rev-campo", "", true)
		if got.Originale != testo {
			t.Errorf("%q: originale %q", testo, got.Originale)
		}
		if r := got.Revisione; r != nil && (r.Stato != StatoRevisioneLetta || r.Normalizzata != testo || r.Regola != "rev-campo" ||
			r.Sorgente != grammatica.SorgenteCampoSeparato || r.Originale != testo || r.Token != nil) {
			t.Errorf("%q: lettura %+v", testo, r)
		}
	}
}

// TestRegistrataTestoNonLetto (D-07; mai «codice + rev» composti): la regola legge il testo intero o niente; un pezzo di
// codice, il codice con la revisione, un testo in più, una cifra sola o gli spazi ai bordi (che toglie chi chiama) danno
// non_interpretabile senza valore, con la regola; il testo vuoto ha il suo motivo.
func TestRegistrataTestoNonLetto(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCampoSeparato()))
	for _, testo := range []string{"01 bis", "1", " 01", "T+300.012345.010", "T+300.012345.010 01", "T+300.012345.010/01"} {
		got := m.LeggiRevisioneRegistrata(famigliaColonna, testo)
		controllaRegistrata(t, testo, got, StatoRevisioneRegistrataNonInterpretabile, famigliaColonna+"/rev-campo",
			MotivoRevisioneRegistrataTestoNonLetto, false)
		if got.Originale != testo {
			t.Errorf("%q: originale %q, l'originale non si tocca", testo, got.Originale)
		}
	}
	controllaRegistrata(t, "vuoto", m.LeggiRevisioneRegistrata(famigliaColonna, ""), StatoRevisioneRegistrataNonInterpretabile,
		famigliaColonna+"/rev-campo", MotivoRevisioneRegistrataTestoVuoto, false)
}

// TestRegistrataNessunaRegola (R113 B): senza regole in campo separato nella famiglia lo stato è nessuna_regola, anche se
// una regola in linea della famiglia leggerebbe lo stesso testo dentro un codice: le regole in linea non contano, e il
// testo non si unisce mai al codice per leggerlo. Anche una famiglia che il motore non conosce, la famiglia vuota e il
// motore nullo.
func TestRegistrataNessunaRegola(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famPunti(), famCampoSeparato()))
	// la regola in linea di acme-marcatore legge «2» dentro «9123456A2»
	if letture, _ := m.Riconosci(evidenze.Selettore{Contesto: evidenze.ContestoCartiglio,
		Campo: evidenze.CampoFonte{Variante: evidenze.VarianteCartiglio, Valore: "codice"}}, "9123456A2"); len(letture) == 0 ||
		letture[0].Revisione == nil || letture[0].Revisione.Normalizzata != "2" {
		t.Fatalf("premessa: la regola in linea non legge la revisione nel codice: %+v", letture)
	}
	for _, c := range []struct{ nome, famiglia, testo string }{
		{"solo regole in linea", "acme-marcatore", "2"},
		{"solo regole in linea, il codice composto", "acme-marcatore", "9123456A2"},
		{"solo regole in linea, con la barra", "acme-punti", "01"},
		{"famiglia che il motore non conosce", "acme-ignota", "01"},
		{"famiglia vuota", "", "01"},
	} {
		controllaRegistrata(t, c.nome, m.LeggiRevisioneRegistrata(c.famiglia, c.testo), StatoRevisioneRegistrataNessunaRegola, "",
			MotivoRevisioneRegistrataRegoleAssenti, false)
	}
	var nullo *Motore
	controllaRegistrata(t, "motore nullo", nullo.LeggiRevisioneRegistrata(famigliaColonna, "01"), StatoRevisioneRegistrataNessunaRegola,
		"", MotivoRevisioneRegistrataRegoleAssenti, false)
}

// TestRegistrataDueRegoleAmbigua (R113 B: «non un'uguaglianza inventata»): due regole attive in campo separato con ID
// diversi danno ambigua senza provarle, anche quando una sola legge il testo; nessuna regola si sceglie.
func TestRegistrataDueRegoleAmbigua(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famColonnaCon(regolaColonna("rev-campo", "[0-9]{2}", "cartiglio.revisione"),
		regolaColonna("rev-lettera", "[A-Z]", "cartiglio.revisione"))))
	for _, testo := range []string{"01", "B", "zz"} {
		controllaRegistrata(t, testo, m.LeggiRevisioneRegistrata(famigliaColonna, testo), StatoRevisioneRegistrataAmbigua, "",
			MotivoRevisioneRegistrataRegoleMultiple, false)
	}
}

// TestRegistrataStessaRegolaSuPiuSelettori: la colonna non ha un selettore, quindi le regole si prendono da tutti i
// selettori, senza doppioni per ID: la stessa regola su due selettori è una regola sola, non un'ambiguità.
func TestRegistrataStessaRegolaSuPiuSelettori(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famColonnaCon(regolaColonna("rev-campo", "[0-9]{2}", "cartiglio.revisione", "corpo"))))
	n := 0
	for _, piani := range m.revisioniCampo {
		n += len(piani)
	}
	if n != 2 {
		t.Fatalf("premessa: la regola sta su %d selettori, attesi 2", n)
	}
	controllaRegistrata(t, "due selettori", m.LeggiRevisioneRegistrata(famigliaColonna, "07"), StatoRevisioneRegistrataLetta,
		famigliaColonna+"/rev-campo", "", true)
}

// TestRegistrataSoloLaFamiglia: valgono le sole regole della famiglia data; quella di un'altra famiglia sullo stesso
// selettore non rende la lettura ambigua e non legge al suo posto. Anche una regola riservata di un'altra famiglia non
// conta: la famiglia senza regole in campo separato resta nessuna_regola (R-143 della revisione di B6b).
func TestRegistrataSoloLaFamiglia(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCampoSeparato(), famLettere()))
	controllaRegistrata(t, "cifre", m.LeggiRevisioneRegistrata(famigliaColonna, "01"), StatoRevisioneRegistrataLetta,
		famigliaColonna+"/rev-campo", "", true)
	controllaRegistrata(t, "lettera", m.LeggiRevisioneRegistrata("acme-lettere", "B"), StatoRevisioneRegistrataLetta,
		"acme-lettere/rev-lettera", "", true)
	controllaRegistrata(t, "cifre con la regola delle lettere", m.LeggiRevisioneRegistrata("acme-lettere", "01"),
		StatoRevisioneRegistrataNonInterpretabile, "acme-lettere/rev-lettera", MotivoRevisioneRegistrataTestoNonLetto, false)
	controllaRegistrata(t, "lettera con la regola delle cifre", m.LeggiRevisioneRegistrata(famigliaColonna, "B"),
		StatoRevisioneRegistrataNonInterpretabile, famigliaColonna+"/rev-campo", MotivoRevisioneRegistrataTestoNonLetto, false)

	lettere := famLettere()
	lettere.Revisioni[0].Stato = grammatica.StatoRiservata
	r, _ := compilaBene(t, grammaticaACME(famMarcatore(), lettere))
	controllaRegistrata(t, "la riservata di un'altra famiglia", r.LeggiRevisioneRegistrata("acme-marcatore", "2"),
		StatoRevisioneRegistrataNessunaRegola, "", MotivoRevisioneRegistrataRegoleAssenti, false)
	controllaRegistrata(t, "la riservata della sua famiglia", r.LeggiRevisioneRegistrata("acme-lettere", "B"),
		StatoRevisioneRegistrataNonInterpretabile, "acme-lettere/rev-lettera", MotivoRevisioneRegistrataRegolaRiservata, false)
}

// TestRegistrataTokenSospeso (D5): la regola legge il testo come un token sospeso: non_interpretabile, con il token
// conservato nella lettura e nessun valore.
func TestRegistrataTokenSospeso(t *testing.T) {
	r := regolaColonna("rev-campo", "[0-9]{2}", "cartiglio.revisione")
	r.TokenSospesi = []grammatica.TokenSospeso{{ID: "Q-ACME-1", Pattern: "xx", Riserva: "Q-ACME-1"}}
	m, _ := compilaBene(t, grammaticaACME(famColonnaCon(r)))
	got := m.LeggiRevisioneRegistrata(famigliaColonna, "xx")
	controllaRegistrata(t, "token", got, StatoRevisioneRegistrataNonInterpretabile, famigliaColonna+"/rev-campo",
		MotivoRevisioneRegistrataTokenSospeso, true)
	if l := got.Revisione; l != nil && (l.Stato != StatoRevisioneNonInterpretabile || l.Normalizzata != "" || l.Token == nil ||
		l.Token.Valore != "xx" || !l.RichiedeVerifica) {
		t.Errorf("token: lettura %+v", l)
	}
	// la stessa regola legge ancora le due cifre
	controllaRegistrata(t, "cifre", m.LeggiRevisioneRegistrata(famigliaColonna, "02"), StatoRevisioneRegistrataLetta,
		famigliaColonna+"/rev-campo", "", true)
}

// TestRegistrataRegolaRiservata (Q1): una famiglia con la sola regola riservata dà non_interpretabile, con la regola e
// senza valore; con più riservate nessuna si nomina; una riservata accanto a un'attiva non conta, come in Interpreta.
func TestRegistrataRegolaRiservata(t *testing.T) {
	riservata := func(id string) grammatica.RegolaRevisione {
		r := regolaColonna(id, "[0-9]{2}", "cartiglio.revisione")
		r.Stato = grammatica.StatoRiservata
		return r
	}
	m, _ := compilaBene(t, grammaticaACME(famColonnaCon(riservata("rev-campo"))))
	controllaRegistrata(t, "una riservata", m.LeggiRevisioneRegistrata(famigliaColonna, "01"), StatoRevisioneRegistrataNonInterpretabile,
		famigliaColonna+"/rev-campo", MotivoRevisioneRegistrataRegolaRiservata, false)

	m, _ = compilaBene(t, grammaticaACME(famColonnaCon(riservata("rev-campo"), riservata("rev-vecchia"))))
	controllaRegistrata(t, "due riservate", m.LeggiRevisioneRegistrata(famigliaColonna, "01"), StatoRevisioneRegistrataNonInterpretabile,
		"", MotivoRevisioneRegistrataRegolaRiservata, false)

	m, _ = compilaBene(t, grammaticaACME(famColonnaCon(regolaColonna("rev-campo", "[0-9]{2}", "cartiglio.revisione"), riservata("rev-vecchia"))))
	controllaRegistrata(t, "un'attiva e una riservata", m.LeggiRevisioneRegistrata(famigliaColonna, "01"), StatoRevisioneRegistrataLetta,
		famigliaColonna+"/rev-campo", "", true)
}

// TestRegistrataDeterministica: lo stesso motore, la stessa famiglia e lo stesso testo danno la stessa revisione a ogni
// chiamata, e le famiglie e le regole permutate nella grammatica danno la stessa: nessun ordine viene da una mappa.
func TestRegistrataDeterministica(t *testing.T) {
	a := famColonnaCon(regolaColonna("rev-campo", "[0-9]{2}", "cartiglio.revisione", "corpo"), regolaColonna("rev-lettera", "[A-Z]", "corpo"))
	b := famColonnaCon(regolaColonna("rev-lettera", "[A-Z]", "corpo"), regolaColonna("rev-campo", "[0-9]{2}", "corpo", "cartiglio.revisione"))
	m1, _ := compilaBene(t, grammaticaACME(a, famLettere()))
	m2, _ := compilaBene(t, grammaticaACME(famLettere(), b))
	for _, c := range []struct{ famiglia, testo string }{{famigliaColonna, "01"}, {"acme-lettere", "B"}, {"acme-lettere", "01"}} {
		primo := m1.LeggiRevisioneRegistrata(c.famiglia, c.testo)
		for i := 0; i < 20; i++ {
			if got := m1.LeggiRevisioneRegistrata(c.famiglia, c.testo); !reflect.DeepEqual(got, primo) {
				t.Fatalf("%s %q: %+v, poi %+v", c.famiglia, c.testo, primo, got)
			}
		}
		if got := m2.LeggiRevisioneRegistrata(c.famiglia, c.testo); !reflect.DeepEqual(got, primo) {
			t.Errorf("%s %q: con la grammatica permutata %+v, prima %+v", c.famiglia, c.testo, got, primo)
		}
	}
}

// TestRegistrataCostantiEFirme (E2 §2.6; T-B0-17, D2): la versione propria, diversa da quelle che non cambiano; i quattro
// stati e i sei motivi, distinti; la firma del metodo e i campi della RevisioneRegistrata, con i tag JSON.
func TestRegistrataCostantiEFirme(t *testing.T) {
	if VersioneRevisioneRegistrata != "revisione-registrata-1" || VersioneRevisioneRegistrata == VersioneAlgoritmo ||
		VersioneRevisioneRegistrata == VersioneComposizione || VersioneAlgoritmo != "motorea-1" || VersioneComposizione != "composizione-1" {
		t.Errorf("versioni %q %q %q", VersioneRevisioneRegistrata, VersioneAlgoritmo, VersioneComposizione)
	}
	stati := map[string]string{
		StatoRevisioneRegistrataLetta: "letta", StatoRevisioneRegistrataNonInterpretabile: "non_interpretabile",
		StatoRevisioneRegistrataNessunaRegola: "nessuna_regola", StatoRevisioneRegistrataAmbigua: "ambigua",
	}
	motivi := map[string]string{
		MotivoRevisioneRegistrataRegoleAssenti: "regole_assenti", MotivoRevisioneRegistrataRegoleMultiple: "regole_multiple",
		MotivoRevisioneRegistrataRegolaRiservata: "regola_riservata", MotivoRevisioneRegistrataTestoVuoto: "testo_vuoto",
		MotivoRevisioneRegistrataTestoNonLetto: "testo_non_letto", MotivoRevisioneRegistrataTokenSospeso: "token_sospeso",
	}
	if len(stati) != 4 || len(motivi) != 6 {
		t.Fatalf("stati %d, motivi %d: non distinti", len(stati), len(motivi))
	}
	for _, m := range []map[string]string{stati, motivi} {
		for k, v := range m {
			if k != v {
				t.Errorf("%q, atteso %q", k, v)
			}
		}
	}
	meth, ok := reflect.TypeFor[*Motore]().MethodByName("LeggiRevisioneRegistrata")
	if !ok || meth.Type.String() != "func(*motorea.Motore, string, string) motorea.RevisioneRegistrata" {
		t.Errorf("firma: %v (%v)", meth.Type, ok)
	}
	var campi []string
	tp := reflect.TypeFor[RevisioneRegistrata]()
	for i := range tp.NumField() {
		campi = append(campi, tp.Field(i).Name+":"+tp.Field(i).Tag.Get("json")+":"+tp.Field(i).Type.String())
	}
	if got := strings.Join(campi, " "); got != "Originale:originale:string Revisione:revisione,omitempty:*motorea.RevisioneLetta "+
		"Stato:stato:string Regola:regola,omitempty:string Motivo:motivo,omitempty:string" {
		t.Errorf("campi: %s", got)
	}
}
