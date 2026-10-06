// L1 — il compositore dei codici (contratto di A1c §2.1, §7 famiglia B3; R63 B, R86, R87; T-B0-17, T-B0-37,
// T-15; PO-06; workflow, passo 5 e B3): dal grezzo letto alla stringa nella forma attiva su cartiglio.codice
// della stessa famiglia, e nessuna stringa, con il motivo, quando la grammatica non la determina in modo
// univoco. Le prove sono tre: l'esempio del workflow (A3.1), un caso per ogni motivo (A3.2) e la verifica
// avversariale (A3.3), una tabella di casi in cui il compositore non deve inventare una stringa. In più: la
// lettura che riceve non cambia, le firme di A1a e A1b sono quelle di prima, il risultato è deterministico.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono quelle ACME di sintetiche_test.go, più famiglie ACME scritte
// qui per il compositore (acme-compositore, acme-maiuscole); basi, letterali ed esempi sono inventati e
// attivano un meccanismo, mai il significato di un cliente (R20 a).
package motorea

import (
	"encoding/json"
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Le firme di A1a e A1b che il compositore usa o affianca restano quelle di prima (D2), e quella nuova è quella
// del contratto (§2.1): se una cambia, il pacchetto di prova non compila.
var (
	_ func(*Motore, LetturaForma) CodiceComposto                                                      = (*Motore).ComponiCodiceDocumentale
	_ func(grammatica.SnapshotRegole, grammatica.Limiti) (*Motore, []evidenze.Diagnostica, error)     = CompilaVerificato
	_ func(*Motore, evidenze.Selettore, string) ([]LetturaForma, []evidenze.Diagnostica)              = (*Motore).Riconosci
	_ func(*Motore, evidenze.DocumentoEvidenze, evidenze.UsoSegmenti) (Interpretazione, error)        = (*Motore).Interpreta
	_ func(BaseLetta, BaseLetta) Compatibilita                                                        = ConfrontaBasi
	_ func(RevisioneLetta, RevisioneLetta) Compatibilita                                              = ConfrontaRevisioni
	_ func(grammatica.IndiceRegole, map[string][]byte) (InsiemeRegole, []evidenze.Diagnostica, error) = CompilaInsieme
)

// ---- gli aiuti ----

// formaEsempio: una forma con il testo del suo esempio positivo (regola 15 di Valida); testo vuoto per una forma
// riservata, che non ne chiede.
type formaEsempio struct {
	f     grammatica.FormaCodice
	testo string
}

func facoltativa(p grammatica.Parte) grammatica.Parte {
	p.Min = 0
	return p
}

func riservata(f grammatica.FormaCodice) grammatica.FormaCodice {
	f.Stato = grammatica.StatoRiservata
	return f
}

// famigliaKit: la famiglia ACME scritta per il compositore.
const famigliaKit = "acme-compositore"

// revisioneKit: una revisione inline della famiglia del compositore.
func revisioneKit(id string, selettori, separatori []string, segmenti ...grammatica.SegmentoRevisione) grammatica.RegolaRevisione {
	return grammatica.RegolaRevisione{ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva,
		Sorgente: grammatica.SorgenteInline, Separatori: separatori, Segmenti: segmenti}
}

// famKit: acme-compositore, base «8» più una cifra fra 1 e 3 e cinque cifre, con le forme del cartiglio date. Le
// forme da cui vengono le letture stanno ognuna su un selettore o su testi che le altre non leggono, così ogni
// esempio ha una lettura sola:
//   - nodo [base][A] e nodo-rev [base][A]_[rev] sui nodi; radice [base][A facoltativo], radice-prt
//     [base][A]_[PRT] e radice-altra [base][A]_[rev-altra] sulle radici;
//   - nome [pacchetto facoltativo][base][A]_[rev] e dxf dxf_[base][a] sul nome del file;
//   - corpo [P facoltativo][base][A][rev] e ripetuta [base]#R[base] sul corpo;
//   - oggetto [base][A][rev-due] e trattino [base][A][rev-separata] sull'oggetto;
//   - solo-base [base] sul testo del PDF, sospesa [base][A]_[rev-sospesa] sulla storia.
//
// Le regole citate dalle forme del cartiglio sono dichiarate anche su cartiglio.codice (regola 7 di Valida): rev,
// rev-due (forte e debole), rev-separata (due separatori), rev-sospesa (con il token «x»), l'affisso P, le
// etichette COD (senza spazi) e PN (fino a uno spazio), le decorazioni pacchetto (un letterale), stato (un pattern)
// e formato (un tipo riservato in A1). rev-altra sta solo sulle radici.
func famKit(cartiglio ...formaEsempio) grammatica.FamigliaCodice {
	rev := grammatica.TipoParteRevisione
	nodo := forma("nodo", sel("nodo_step.id"), pBase(), pMarcatore("A"))
	nodo.ConfineDopo = classe(grammatica.ConfineParolaASCII)
	radice := forma("radice", sel("radice_step.id"), pBase(), facoltativa(pMarcatore("A")))
	radice.ConfineDopo = classe(grammatica.ConfineParolaASCII)
	sorgenti := []formaEsempio{
		{nodo, "8123456A"},
		{forma("nodo-rev", sel("nodo_step.id"), pBase(), pMarcatore("A"), pSep("_"), pRif(rev, "rev", 1)), "8123456A_2"},
		{radice, "8123456"},
		{forma("radice-prt", sel("radice_step.id"), pBase(), pMarcatore("A"), pSep("_"), pToken("D10", "PRT")), "8123456A_PRT"},
		{forma("radice-altra", sel("radice_step.id"), pBase(), pMarcatore("A"), pSep("_"), pRif(rev, "rev-altra", 1)), "8123456A_3"},
		{forma("nome", sel("nome_file"), pRif(grammatica.TipoParteDecorazione, "pacchetto", 0), pBase(), pMarcatore("A"), pSep("_"),
			pRif(rev, "rev", 1)), "8123456A_2.stp"},
		{forma("dxf", sel("nome_file"), pSep("dxf_"), pBase(), pMarcatore("a")), "dxf_8123456a.dxf"},
		{forma("corpo", sel("corpo"), pRif(grammatica.TipoParteAffisso, "P", 0), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "P8123456A2"},
		{forma("ripetuta", sel("corpo"), pBase(), pSep("#R"), pRipetizione()), "8123456#R8123456"},
		{forma("oggetto", sel("oggetto"), pBase(), pMarcatore("A"), pRif(rev, "rev-due", 1)), "8123456A01"},
		{forma("trattino", sel("oggetto"), pBase(), pMarcatore("A"), pRif(rev, "rev-separata", 1)), "8123456A-2"},
		{forma("solo-base", sel("testo_pdf"), pBase()), "8123456"},
		{forma("sospesa", sel("storia"), pBase(), pMarcatore("A"), pSep("_"), pRif(rev, "rev-sospesa", 1)), "8123456A_3"},
	}
	valore := grammatica.SegmentoRevisione{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}
	f := grammatica.FamigliaCodice{
		ID: famigliaKit, Namespace: famigliaKit,
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("numero", "8[1-3][0-9]{5}", "")),
		Affissi: []grammatica.Affisso{{ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: sel("cartiglio.codice", "corpo"), Attribuzione: sel("cartiglio.codice", "corpo"),
			Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo}}},
		Etichette: []grammatica.Etichetta{
			{ID: "cod", Letterali: []string{"COD"}, SpaziMax: 0, Selettori: sel("cartiglio.codice")},
			{ID: "pn", Letterali: []string{"PN"}, SpaziMax: 1, Selettori: sel("cartiglio.codice")},
		},
		Revisioni: []grammatica.RegolaRevisione{
			revisioneKit("rev", sel("cartiglio.codice", "corpo", "nodo_step.id", "nome_file"), nil, valore),
			revisioneKit("rev-altra", sel("radice_step.id"), nil, valore),
			revisioneKit("rev-due", sel("cartiglio.codice", "oggetto"), nil,
				grammatica.SegmentoRevisione{Nome: "forte", Pattern: "[0-9]", Significato: grammatica.SignificatoForte},
				grammatica.SegmentoRevisione{Nome: "debole", Pattern: "[0-9]", Significato: grammatica.SignificatoDebole}),
			revisioneKit("rev-separata", sel("cartiglio.codice", "oggetto"), []string{"-", "/"}, valore),
			func() grammatica.RegolaRevisione {
				r := revisioneKit("rev-sospesa", sel("cartiglio.codice", "storia"), nil, valore)
				r.TokenSospesi = []grammatica.TokenSospeso{{ID: "x", Pattern: "x", Riserva: "Q-ACME-1"}}
				return r
			}(),
		},
		Decorazioni: []grammatica.Decorazione{
			{ID: "formato", Tipo: grammatica.TipoDecorazioneFormato, Letterali: []string{"A4"}, Selettori: sel("cartiglio.codice")},
			{ID: "pacchetto", Tipo: grammatica.TipoDecorazioneInvolucro, Sottotipo: grammatica.SottotipoRiferimentoPacchetto,
				Letterali: []string{"ACME-030"}, Selettori: sel("cartiglio.codice", "nome_file")},
			{ID: "stato", Tipo: grammatica.TipoDecorazioneStatoPDM, Pattern: "[A-Z]{2}", Selettori: sel("cartiglio.codice")},
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-nome-pacchetto", "nome_file", "ACME-0308123456A_2.stp", grammatica.LetturaAttesa{Forma: "nome", Decorazioni: []string{"ACME-030"}}),
		},
	}
	for _, fe := range append(sorgenti, cartiglio...) {
		f.Forme = append(f.Forme, fe.f)
		if fe.testo != "" {
			f.Esempi = append(f.Esempi, positivo("e-"+fe.f.ID, fe.f.Selettori[0], fe.testo, grammatica.LetturaAttesa{Forma: fe.f.ID}))
		}
	}
	return f
}

// cartiglioStd: [base][A][rev], la forma del cartiglio con la revisione obbligatoria.
func cartiglioStd() formaEsempio {
	return formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"),
		pRif(grammatica.TipoParteRevisione, "rev", 1)), "8123456A2"}
}

// famMarcatoreNodo: acme-marcatore con una forma in più sui nodi STEP, [base][A]_[rev-a], perché l'esempio del
// workflow si possa leggere anche da un nodo (nella famiglia di sintetiche_test.go i nodi leggono solo [base][A]).
func famMarcatoreNodo() grammatica.FamigliaCodice {
	f := famMarcatore()
	f.Revisioni[0].Selettori = append(f.Revisioni[0].Selettori, "nodo_step.id")
	f.Forme = append(f.Forme, forma("step-rev", sel("nodo_step.id"), pBase(), pMarcatore("A"), pSep("_"),
		pRif(grammatica.TipoParteRevisione, "rev-a", 1)))
	f.Esempi = append(f.Esempi, positivo("e-step-rev", "nodo_step.id", "9123456A_2",
		grammatica.LetturaAttesa{Forma: "step-rev", Base: "9123456", Marcatore: "A", Revisione: "2"}))
	return f
}

// famPuntiCartiglio: acme-punti (base a punti, revisione «/nn» con forte e debole e il token «xx», forma
// parziale senza T) con una forma sul cartiglio; rev-barra vale anche lì.
func famPuntiCartiglio(cartiglio formaEsempio) grammatica.FamigliaCodice {
	f := famPunti()
	for i := range f.Revisioni {
		if f.Revisioni[i].ID == "rev-barra" {
			f.Revisioni[i].Selettori = append(f.Revisioni[i].Selettori, "cartiglio.codice")
		}
	}
	f.Forme = append(f.Forme, cartiglio.f)
	f.Esempi = append(f.Esempi, positivo("e-"+cartiglio.f.ID, "cartiglio.codice", cartiglio.testo, grammatica.LetturaAttesa{Forma: cartiglio.f.ID}))
	return f
}

// famMaiuscole: acme-maiuscole, una base di due lettere e quattro cifre con le maiuscole e la normalizzazione
// date; [base] sul cartiglio e sui nodi.
func famMaiuscole(maiuscole, normalizza, pattern, esempio string) grammatica.FamigliaCodice {
	b := base(segPattern("codice", pattern, ""))
	b.Maiuscole, b.Normalizza = maiuscole, normalizza
	return grammatica.FamigliaCodice{
		ID: "acme-maiuscole", Namespace: "acme-maiuscole",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  b,
		Forme: []grammatica.FormaCodice{
			forma("cartiglio", sel("cartiglio.codice"), pBase()),
			forma("nodo", sel("nodo_step.id"), pBase()),
		},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-cartiglio", "cartiglio.codice", esempio, grammatica.LetturaAttesa{Forma: "cartiglio"}),
			positivo("e-nodo", "nodo_step.id", esempio, grammatica.LetturaAttesa{Forma: "nodo"}),
		},
	}
}

// leggiUna: l'unica lettura di quella famiglia e forma sul testo, con gli invarianti di riconosci.
func leggiUna(t *testing.T, m *Motore, s, testo, famiglia, nomeForma string) LetturaForma {
	t.Helper()
	letture, _ := riconosci(t, m, s, testo)
	var trovate []LetturaForma
	for _, l := range letture {
		if l.Famiglia == famiglia && l.Forma == nomeForma {
			trovate = append(trovate, l)
		}
	}
	if len(trovate) != 1 {
		t.Fatalf("%s %q: attesa una lettura %s/%s, trovate %s", s, testo, famiglia, nomeForma, riassunto(letture))
	}
	return trovate[0]
}

func copiaSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append(make([]T, 0, len(s)), s...)
}

func copiaParteLetta(p *ParteLetta) *ParteLetta {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func copiaBaseLetta(b BaseLetta) BaseLetta {
	b.Segmenti = copiaSlice(b.Segmenti)
	b.Mancanti = copiaSlice(b.Mancanti)
	return b
}

// copiaLettura: una copia profonda, per controllare che il compositore non cambi la lettura che riceve.
func copiaLettura(l LetturaForma) LetturaForma {
	c := l
	c.Base = copiaBaseLetta(l.Base)
	c.Marcatore, c.Token, c.Etichetta = copiaParteLetta(l.Marcatore), copiaParteLetta(l.Token), copiaParteLetta(l.Etichetta)
	c.Affissi = copiaSlice(l.Affissi)
	for i := range c.Affissi {
		if v := c.Affissi[i].Valore; v != nil {
			x := *v
			c.Affissi[i].Valore = &x
		}
	}
	if l.Revisione != nil {
		r := *l.Revisione
		r.Segmenti = copiaSlice(r.Segmenti)
		r.Token = copiaParteLetta(r.Token)
		r.equivalenze = copiaSlice(r.equivalenze)
		c.Revisione = &r
	}
	c.Decorazioni = copiaSlice(l.Decorazioni)
	c.Ripetizioni = copiaSlice(l.Ripetizioni)
	for i := range c.Ripetizioni {
		c.Ripetizioni[i].Base = copiaBaseLetta(c.Ripetizioni[i].Base)
	}
	c.Categorie = copiaSlice(l.Categorie)
	return c
}

// componiControllato: compone e controlla gli invarianti di ogni risultato. La lettura non cambia; una seconda
// chiamata dà lo stesso risultato; testo e motivo si escludono; il motivo è uno dei sette del contratto; la
// famiglia è quella della lettura.
func componiControllato(t *testing.T, m *Motore, l LetturaForma) CodiceComposto {
	t.Helper()
	prima := copiaLettura(l)
	c := m.ComponiCodiceDocumentale(l)
	if !reflect.DeepEqual(l, prima) {
		t.Fatalf("il compositore ha cambiato la lettura:\nprima %+v\ndopo  %+v", prima, l)
	}
	if di := m.ComponiCodiceDocumentale(l); di != c {
		t.Fatalf("due chiamate, due risultati: %+v e %+v", c, di)
	}
	if (c.Testo == "") == (c.Motivo == "") {
		t.Fatalf("testo %q e motivo %q: uno e uno solo dei due", c.Testo, c.Motivo)
	}
	if c.Motivo != "" && !contiene(motiviDelContratto(), c.Motivo) {
		t.Fatalf("motivo %q fuori dall'elenco del contratto", c.Motivo)
	}
	if c.Famiglia != l.Famiglia {
		t.Fatalf("famiglia %q, la lettura è di %q", c.Famiglia, l.Famiglia)
	}
	return c
}

// motiviDelContratto: i sette motivi del contratto di A1c, §2.1.
func motiviDelContratto() []string {
	return []string{MotivoComposizioneFormaAssente, MotivoComposizioneFormeMultiple, MotivoComposizioneParteNonDeterminata,
		MotivoComposizioneQualificatoreNonTrasferibile, MotivoComposizioneRevisioneNonDeterminata,
		MotivoComposizioneRevisioneAmbigua, MotivoComposizioneLetturaNonCompleta}
}

// casoLettura: una lettura da prendere con Riconosci e il risultato atteso del compositore.
type casoLettura struct {
	sel, testo, forma string
	atteso, motivo    string
}

// controllaCasi: per ogni caso, la lettura della famiglia e il CodiceComposto atteso, con la forma del cartiglio
// usata (vuota se la famiglia non ne ha una sola).
func controllaCasi(t *testing.T, m *Motore, famiglia, formaUsata string, casi []casoLettura) {
	t.Helper()
	for _, cs := range casi {
		l := leggiUna(t, m, cs.sel, cs.testo, famiglia, cs.forma)
		got := componiControllato(t, m, l)
		want := CodiceComposto{Testo: cs.atteso, Famiglia: famiglia, Forma: formaUsata, Motivo: cs.motivo}
		if got != want {
			t.Errorf("%s %q (%s/%s): %+v, atteso %+v", cs.sel, cs.testo, famiglia, cs.forma, got, want)
		}
	}
}

// ---- le costanti del contratto ----

// TestComponiCostantiDelContratto (contratto A1c §2.1; T-B0-17, T-B0-37): i sette motivi con i valori del
// contratto, tutti diversi, revisione_non_determinata distinto da revisione_ambigua; la versione propria del
// compositore, diversa da quella del riconoscimento, che non cambia; il selettore del cartiglio.
func TestComponiCostantiDelContratto(t *testing.T) {
	attesi := map[string]string{
		MotivoComposizioneFormaAssente:                 "forma_cartiglio_assente",
		MotivoComposizioneFormeMultiple:                "forme_cartiglio_multiple",
		MotivoComposizioneParteNonDeterminata:          "parte_non_determinata",
		MotivoComposizioneQualificatoreNonTrasferibile: "qualificatore_non_trasferibile",
		MotivoComposizioneRevisioneNonDeterminata:      "revisione_non_determinata",
		MotivoComposizioneRevisioneAmbigua:             "revisione_ambigua",
		MotivoComposizioneLetturaNonCompleta:           "lettura_non_completa",
	}
	if len(attesi) != 7 || len(motiviDelContratto()) != 7 {
		t.Fatalf("i motivi non sono sette e distinti: %v", attesi)
	}
	for k, v := range attesi {
		if k != v {
			t.Errorf("motivo %q, il contratto dice %q", k, v)
		}
	}
	if VersioneComposizione != "composizione-1" || VersioneComposizione == VersioneAlgoritmo || VersioneAlgoritmo != "motorea-1" {
		t.Errorf("VersioneComposizione %q, VersioneAlgoritmo %q: attese composizione-1 e motorea-1", VersioneComposizione, VersioneAlgoritmo)
	}
	s, err := evidenze.LeggiSelettore("cartiglio.codice")
	if err != nil || s != selettoreCartiglioCodice {
		t.Errorf("selettore del cartiglio %+v, LeggiSelettore dà %+v (%v)", selettoreCartiglioCodice, s, err)
	}
}

// TestComponiNessunCampoCambiatoNelleLetture (T-B0-17, D2): i campi di LetturaForma e di LetturaCodice sono
// quelli di A1a e A1b, nello stesso ordine: il compositore non ne aggiunge e non ne toglie.
func TestComponiNessunCampoCambiatoNelleLetture(t *testing.T) {
	campi := func(v any) []string {
		var out []string
		tp := reflect.TypeOf(v)
		for i := 0; i < tp.NumField(); i++ {
			out = append(out, tp.Field(i).Name)
		}
		return out
	}
	attesi := map[string][]string{
		"LetturaForma": {"Famiglia", "Namespace", "Forma", "Selettore", "Originale", "Intervallo", "CodiceRichiesto", "Base",
			"Marcatore", "Token", "Etichetta", "Affissi", "Revisione", "Decorazioni", "Ripetizioni", "Categorie", "Stato"},
		"LetturaCodice": {"ID", "UnitaID", "AltreUnita", "Occorrenza", "Assoluto", "Forma", "Funzione", "Uso", "OrigineUso",
			"RuoliCandidati", "MotivoRuoli", "Categorie", "Trasformazioni", "Qualita", "Motivi"},
		"CodiceComposto": {"Testo", "Famiglia", "Forma", "Motivo"},
	}
	for nome, v := range map[string]any{"LetturaForma": LetturaForma{}, "LetturaCodice": LetturaCodice{}, "CodiceComposto": CodiceComposto{}} {
		if got := campi(v); !reflect.DeepEqual(got, attesi[nome]) {
			t.Errorf("%s: campi %v, attesi %v", nome, got, attesi[nome])
		}
	}
}

// TestComponiJSONDelCodiceComposto: i nomi JSON del CodiceComposto, in snake_case come i tipi di A1c; testo
// sempre presente (vuoto vuol dire non determinato), forma e motivo solo se ci sono.
func TestComponiJSONDelCodiceComposto(t *testing.T) {
	for _, cs := range []struct {
		c    CodiceComposto
		json string
	}{
		{CodiceComposto{Testo: "9123456A2", Famiglia: "acme-marcatore", Forma: "cartiglio"}, `{"testo":"9123456A2","famiglia":"acme-marcatore","forma":"cartiglio"}`},
		{CodiceComposto{Famiglia: "acme-marcatore-x", Motivo: MotivoComposizioneFormaAssente}, `{"testo":"","famiglia":"acme-marcatore-x","motivo":"forma_cartiglio_assente"}`},
	} {
		b, err := json.Marshal(cs.c)
		if err != nil || string(b) != cs.json {
			t.Errorf("%+v: %s (%v), atteso %s", cs.c, b, err, cs.json)
		}
	}
}

// ---- A3.1: l'esempio del workflow ----

// TestComponiEsempioDelWorkflow (workflow, B3 A3.1; 6.0.6; R63 B): il grezzo «9123456A_2» si legge come base
// 9123456, marcatore A, revisione 2; la forma attiva della famiglia su cartiglio.codice è [base][A][rev-a]; il
// codice proposto è «9123456A2». Dal nome del file (forma nome-sottolineato) e da un nodo STEP che porta la
// revisione (la forma step-rev della famiglia di prova). Il grezzo non cambia: la lettura resta quella.
func TestComponiEsempioDelWorkflow(t *testing.T) {
	casi := []struct {
		nome    string
		fam     grammatica.FamigliaCodice
		sel     string
		testo   string
		lettaDa string
	}{
		{"dal nome del file", famMarcatore(), "nome_file", "9123456A_2.stp", "nome-sottolineato"},
		{"da un nodo STEP con la revisione", famMarcatoreNodo(), "nodo_step.id", "9123456A_2", "step-rev"},
	}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			m, _ := compilaBene(t, grammaticaACME(cs.fam))
			l := leggiUna(t, m, cs.sel, cs.testo, "acme-marcatore", cs.lettaDa)
			// passo 2: la lettura grammaticale
			if l.Base.Normalizzata != "9123456" || l.Marcatore == nil || l.Marcatore.Valore != "A" || l.Revisione == nil ||
				l.Revisione.Stato != StatoRevisioneLetta || l.Revisione.Normalizzata != "2" || l.Stato != StatoCompleta {
				t.Fatalf("lettura %+v: attese base 9123456, marcatore A, revisione 2, completa", l)
			}
			// passi 3 e 4: la forma del cartiglio e il codice proposto
			got := componiControllato(t, m, l)
			want := CodiceComposto{Testo: "9123456A2", Famiglia: "acme-marcatore", Forma: "cartiglio"}
			if got != want {
				t.Fatalf("%+v, atteso %+v", got, want)
			}
			// il grezzo resta il grezzo (I-6): la lettura conserva l'originale con il trattino basso
			if l.Originale != "9123456A_2" {
				t.Errorf("originale %q: il grezzo non si rinomina", l.Originale)
			}
		})
	}
}

// ---- PO-06 ----

// TestPO06RevisioneNonDeterminataNessunaStringa (PO-06; R87; T-B0-37; LD-14): un nodo STEP che legge base e
// marcatore senza revisione ha una lettura «completa», ma la forma del cartiglio chiede la revisione: nessuna
// stringa, motivo revisione_non_determinata, nessuna revisione inventata (la lettura resta senza). Vale per i nodi
// e per le radici, anche con il token PRT. La revisione del nome del file STEP non entra: è un'altra lettura, un
// candidato della radice che mostra B4; il compositore non ha un ingresso per lei e non tiene stato fra due
// chiamate, quindi il nodo resta senza stringa anche dopo aver composto il nome del file.
func TestPO06RevisioneNonDeterminataNessunaStringa(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore()))
	parziale := CodiceComposto{Famiglia: "acme-marcatore", Forma: "cartiglio", Motivo: MotivoComposizioneRevisioneNonDeterminata}

	nodo := leggiUna(t, m, "nodo_step.id", "9123456A", "acme-marcatore", "step")
	if nodo.Stato != StatoCompleta || nodo.Revisione != nil || nodo.Base.Normalizzata != "9123456" || nodo.Marcatore == nil {
		t.Fatalf("il nodo non è quello di PO-06 (completo, base e marcatore, senza revisione): %+v", nodo)
	}
	if got := componiControllato(t, m, nodo); got != parziale {
		t.Fatalf("nodo senza revisione: %+v, atteso %+v", got, parziale)
	}
	if got := componiControllato(t, m, nodo); got.Motivo == MotivoComposizioneRevisioneAmbigua {
		t.Fatal("revisione assente scambiata per ambigua (LD-14)")
	}
	for _, r := range []struct{ testo, forma string }{{"9123456A", "step"}, {"9123456A_PRT", "step-prt"}} {
		radice := leggiUna(t, m, "radice_step.id", r.testo, "acme-marcatore", r.forma)
		if got := componiControllato(t, m, radice); got != parziale {
			t.Errorf("radice %q: %+v, atteso %+v", r.testo, got, parziale)
		}
	}

	// Il nome del file dello STEP: la sua lettura porta la revisione e si compone per sé.
	nome := leggiUna(t, m, "nome_file", "9123456A_2.STP", "acme-marcatore", "nome-sottolineato")
	if got := componiControllato(t, m, nome); got.Testo != "9123456A2" {
		t.Fatalf("nome del file: %+v, atteso 9123456A2", got)
	}
	if got := componiControllato(t, m, nodo); got != parziale || nodo.Revisione != nil {
		t.Fatalf("il nodo dopo il nome del file: %+v, revisione %v; atteso %+v senza revisione", got, nodo.Revisione, parziale)
	}
}

// ---- A3.2: un caso per ogni motivo ----

// TestComponiUnCasoPerMotivo (workflow, B3 A3.2; contratto A1c §2.1): forma del cartiglio assente, due forme
// possibili, parte mancante, qualificatore STEP non trasferibile, revisione ambigua, lettura non completa (la
// revisione non determinata è PO-06). Ogni caso viene da una lettura vera di Riconosci su una grammatica ACME.
func TestComponiUnCasoPerMotivo(t *testing.T) {
	t.Run(MotivoComposizioneFormaAssente, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
		controllaCasi(t, m, "acme-marcatore-x", "", []casoLettura{
			{"nodo_step.id", "912345X1", "x-nodo", "", MotivoComposizioneFormaAssente},
		})
	})
	t.Run(MotivoComposizioneFormeMultiple, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famKit(cartiglioStd(),
			formaEsempio{forma("cartiglio-trattino", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pSep("-"),
				pRif(grammatica.TipoParteRevisione, "rev", 1)), "8123456A-2"})))
		controllaCasi(t, m, famigliaKit, "", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneFormeMultiple},
		})
	})
	t.Run(MotivoComposizioneParteNonDeterminata, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famKit(formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A")), "8123456A"})))
		controllaCasi(t, m, famigliaKit, "cartiglio", []casoLettura{
			{"testo_pdf", "8123456", "solo-base", "", MotivoComposizioneParteNonDeterminata},
		})
	})
	t.Run(MotivoComposizioneQualificatoreNonTrasferibile, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
		controllaCasi(t, m, "acme-prefisso", "cartiglio", []casoLettura{
			{"corpo", "P7120100", "mail", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"nome_file", "ACME-030P7120100 00 IN_WORK.stp", "nome-step", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"cartiglio.codice", "7120100", "cartiglio", "7120100", ""},
		})
		// La radice STEP con il token PRT, che il cartiglio [base][A] non ha: senza il token sarebbe un altro codice.
		mk, _ := compilaBene(t, grammaticaACME(famKit(formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A")), "8123456A"})))
		controllaCasi(t, mk, famigliaKit, "cartiglio", []casoLettura{
			{"radice_step.id", "8123456A_PRT", "radice-prt", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"nodo_step.id", "8123456A", "nodo", "8123456A", ""},
		})
	})
	t.Run(MotivoComposizioneRevisioneAmbigua, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famKit(cartiglioStd())))
		controllaCasi(t, m, famigliaKit, "cartiglio", []casoLettura{
			{"storia", "8123456A_x", "sospesa", "", MotivoComposizioneRevisioneAmbigua},
		})
	})
	t.Run(MotivoComposizioneLetturaNonCompleta, func(t *testing.T) {
		m, _ := compilaBene(t, grammaticaACME(famPuntiCartiglio(formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase()), "9.123.4567.3"})))
		controllaCasi(t, m, "acme-punti", "cartiglio", []casoLettura{
			{"corpo", "9.123.4567", "parziale", "", MotivoComposizioneLetturaNonCompleta},
		})
		mk, _ := compilaBene(t, grammaticaACME(famKit(cartiglioStd())))
		controllaCasi(t, mk, famigliaKit, "cartiglio", []casoLettura{
			{"corpo", "8123456#R8123457", "ripetuta", "", MotivoComposizioneLetturaNonCompleta},
		})
	})
}

// ---- A3.3: la verifica avversariale ----

// TestComponiNonInventaSuLeFormeDelCartiglio (workflow, B3 A3.3; R63 B, R87): la stessa famiglia con forme del
// cartiglio diverse, e per ognuna le letture che la mettono alla prova. La stringa c'è solo quando ogni parte
// della forma è determinata dalla lettura o da un testo solo, e ogni parte d'identità letta ha il suo posto:
//   - parti facoltative: un marcatore, un affisso o un token facoltativo senza valore resta fuori solo se
//     l'assenza è letta (la forma della lettura aveva quel posto); la revisione facoltativa no (R87);
//   - separatori: fanno parte del codice, un testo solo, mai preso dalla lettura;
//   - etichette e decorazioni della forma: fuori dall'identità, non entrano nella stringa proposta, quindi una
//     forma che le ha non dà stringa (T-B3-02);
//   - maiuscole: i letterali del marcatore sono esatti;
//   - revisione: con gli zeri com'è (D4), della stessa regola, con il separatore della regola se è uno solo;
//   - una forma riservata, o con una parte riservata, non conta.
func TestComponiNonInventaSuLeFormeDelCartiglio(t *testing.T) {
	rev := grammatica.TipoParteRevisione
	scenari := []struct {
		nome      string
		cartiglio []formaEsempio
		usata     string
		casi      []casoLettura
	}{
		{"revisione obbligatoria [base][A][rev]", []formaEsempio{cartiglioStd()}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "8123456A2", ""},
			{"nome_file", "ACME-0308123456A_2.stp", "nome", "8123456A2", ""}, // la decorazione è fuori dal codice
			{"nodo_step.id", "8123456A_2", "nodo-rev", "8123456A2", ""},
			{"corpo", "8123456A2", "corpo", "8123456A2", ""},
			{"cartiglio.codice", "8123456A2", "cartiglio", "8123456A2", ""},
			{"corpo", "P8123456A2", "corpo", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneRevisioneNonDeterminata},
			{"radice_step.id", "8123456", "radice", "", MotivoComposizioneRevisioneNonDeterminata},
			{"radice_step.id", "8123456A_PRT", "radice-prt", "", MotivoComposizioneRevisioneNonDeterminata},
			{"testo_pdf", "8123456", "solo-base", "", MotivoComposizioneRevisioneNonDeterminata},
			{"nome_file", "dxf_8123456a.dxf", "dxf", "", MotivoComposizioneRevisioneNonDeterminata},
			{"corpo", "8123456#R8123456", "ripetuta", "", MotivoComposizioneRevisioneNonDeterminata},
			{"radice_step.id", "8123456A_3", "radice-altra", "", MotivoComposizioneParteNonDeterminata}, // un'altra regola
			{"oggetto", "8123456A01", "oggetto", "", MotivoComposizioneParteNonDeterminata},
			{"storia", "8123456A_3", "sospesa", "", MotivoComposizioneParteNonDeterminata},
			{"storia", "8123456A_x", "sospesa", "", MotivoComposizioneRevisioneAmbigua},
			{"corpo", "8123456#R8123457", "ripetuta", "", MotivoComposizioneLetturaNonCompleta},
		}},
		{"senza revisione [base][A]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A")), "8123456A"}}, "cartiglio", []casoLettura{
			{"nodo_step.id", "8123456A", "nodo", "8123456A", ""},
			{"radice_step.id", "8123456A", "radice", "8123456A", ""},
			{"radice_step.id", "8123456", "radice", "", MotivoComposizioneParteNonDeterminata}, // l'assenza letta non riempie una parte obbligatoria
			{"testo_pdf", "8123456", "solo-base", "", MotivoComposizioneParteNonDeterminata},
			{"nome_file", "dxf_8123456a.dxf", "dxf", "", MotivoComposizioneParteNonDeterminata}, // «a» non è «A»
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"radice_step.id", "8123456A_PRT", "radice-prt", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"storia", "8123456A_x", "sospesa", "", MotivoComposizioneRevisioneAmbigua},
		}},
		{"solo la base [base]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase()), "8123456"}}, "cartiglio", []casoLettura{
			{"testo_pdf", "8123456", "solo-base", "8123456", ""},
			{"radice_step.id", "8123456", "radice", "8123456", ""},
			{"corpo", "8123456#R8123456", "ripetuta", "8123456", ""},
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneQualificatoreNonTrasferibile},
		}},
		{"marcatore facoltativo [base][A?]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), facoltativa(pMarcatore("A"))), "8123456A"}}, "cartiglio", []casoLettura{
			{"nodo_step.id", "8123456A", "nodo", "8123456A", ""},
			{"radice_step.id", "8123456A", "radice", "8123456A", ""},
			{"radice_step.id", "8123456", "radice", "8123456", ""},
			{"cartiglio.codice", "8123456", "cartiglio", "8123456", ""},
			{"testo_pdf", "8123456", "solo-base", "", MotivoComposizioneParteNonDeterminata}, // l'assenza non è letta
		}},
		{"revisione facoltativa [base][A][rev?]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev", 0)), "8123456A2"}}, "cartiglio", []casoLettura{
			{"nodo_step.id", "8123456A_2", "nodo-rev", "8123456A2", ""},
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneRevisioneNonDeterminata},
			{"cartiglio.codice", "8123456A", "cartiglio", "", MotivoComposizioneRevisioneNonDeterminata},
		}},
		{"due separatori possibili [base][-|/][A]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pSep("-", "/"), pMarcatore("A")), "8123456-A"}}, "cartiglio", []casoLettura{
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"un separatore [base]-[A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pSep("-"), pMarcatore("A"), pRif(rev, "rev", 1)), "8123456-A2"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "8123456-A2", ""},
		}},
		{"etichetta con uno spazio ammesso [PN][base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pRif(grammatica.TipoParteEtichetta, "pn", 1), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "PN 8123456A2"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"etichetta senza spazi [COD][base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pRif(grammatica.TipoParteEtichetta, "cod", 1), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "COD8123456A2"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata}, // anche con un testo solo (T-B3-02)
		}},
		{"decorazione con un letterale [base][A][rev]-[pacchetto]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev", 1), pSep("-"), pRif(grammatica.TipoParteDecorazione, "pacchetto", 1)), "8123456A2-ACME-030"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata}, // anche con un testo solo (T-B3-02)
			{"nome_file", "ACME-0308123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"decorazione con un pattern [base][A][rev] [stato]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev", 1), pSep(" "), pRif(grammatica.TipoParteDecorazione, "stato", 1)), "8123456A2 XY"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"decorazione facoltativa [pacchetto?][base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pRif(grammatica.TipoParteDecorazione, "pacchetto", 0), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "ACME-0308123456A2"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
			{"nome_file", "ACME-0308123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata}, // la decorazione letta non si trasferisce
		}},
		{"affisso obbligatorio [P][base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pRif(grammatica.TipoParteAffisso, "P", 1), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "P8123456A2"}}, "cartiglio", []casoLettura{
			{"corpo", "P8123456A2", "corpo", "P8123456A2", ""},
			{"corpo", "8123456A2", "corpo", "", MotivoComposizioneParteNonDeterminata},
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"affisso facoltativo [P?][base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pRif(grammatica.TipoParteAffisso, "P", 0), pBase(), pMarcatore("A"), pRif(rev, "rev", 1)), "P8123456A2"}}, "cartiglio", []casoLettura{
			{"corpo", "P8123456A2", "corpo", "P8123456A2", ""},
			{"corpo", "8123456A2", "corpo", "8123456A2", ""},                                   // l'assenza è letta: la forma del corpo ha la P facoltativa
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata}, // il nome del file non ha posto per la P
		}},
		{"revisione forte e debole [base][A][rev-due]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev-due", 1)), "8123456A01"}}, "cartiglio", []casoLettura{
			{"oggetto", "8123456A01", "oggetto", "8123456A01", ""},
			{"oggetto", "8123456A00", "oggetto", "8123456A00", ""},
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"revisione con due separatori [base][A][rev-separata]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev-separata", 1)), "8123456A-2"}}, "cartiglio", []casoLettura{
			{"oggetto", "8123456A-2", "trattino", "", MotivoComposizioneParteNonDeterminata}, // anche se la lettura ha usato «-»
		}},
		{"revisione con un token sospeso [base][A][rev-sospesa]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev-sospesa", 1)), "8123456A3"}}, "cartiglio", []casoLettura{
			{"storia", "8123456A_3", "sospesa", "8123456A3", ""},
			{"storia", "8123456A_x", "sospesa", "", MotivoComposizioneRevisioneAmbigua},
		}},
		{"token obbligatorio [base][A]_[PRT]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pSep("_"), pToken("D10", "PRT")), "8123456A_PRT"}}, "cartiglio", []casoLettura{
			{"radice_step.id", "8123456A_PRT", "radice-prt", "8123456A_PRT", ""},
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneParteNonDeterminata}, // un token che la lettura non ha non si scrive
		}},
		{"token facoltativo [base][A][PRT?]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), facoltativa(pToken("D10", "PRT"))), "8123456APRT"}}, "cartiglio", []casoLettura{
			{"radice_step.id", "8123456A_PRT", "radice-prt", "8123456APRT", ""},
			{"nodo_step.id", "8123456A", "nodo", "", MotivoComposizioneParteNonDeterminata},
		}},
		{"base ripetuta [base]#R[base][A][rev]", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pSep("#R"), pRipetizione(), pMarcatore("A"), pRif(rev, "rev", 1)), "8123456#R8123456A2"}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "8123456#R8123456A2", ""},
		}},
		{"una attiva e una riservata", []formaEsempio{cartiglioStd(), {riservata(forma("cartiglio-barra", sel("cartiglio.codice"), pBase(), pSep("/"), pMarcatore("A"), pRif(rev, "rev", 1))), ""}}, "cartiglio", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "8123456A2", ""},
		}},
		{"la sola forma usa una decorazione riservata", []formaEsempio{{forma("cartiglio", sel("cartiglio.codice"), pBase(), pMarcatore("A"), pRif(rev, "rev", 1), pRif(grammatica.TipoParteDecorazione, "formato", 1)), "8123456A2A4"}}, "", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneFormaAssente},
		}},
		{"nessuna forma sul cartiglio", nil, "", []casoLettura{
			{"nome_file", "8123456A_2.stp", "nome", "", MotivoComposizioneFormaAssente},
		}},
	}
	for _, sc := range scenari {
		t.Run(sc.nome, func(t *testing.T) {
			m, _ := compilaBene(t, grammaticaACME(famKit(sc.cartiglio...)))
			controllaCasi(t, m, famigliaKit, sc.usata, sc.casi)
		})
	}
}

// TestComponiNonInventaConLaBaseAPunti (A3.3; A-C07, D4, D5): la base a segmenti con il separatore «.». Una lettura
// parziale non dà stringa; la proiezione del cartiglio non è il codice; la revisione «/01» passa con gli zeri e
// con il suo separatore; il token sospeso «xx» blocca anche se il cartiglio non ha la revisione; un affisso
// riconosciuto e non attribuito (la S) non si toglie.
func TestComponiNonInventaConLaBaseAPunti(t *testing.T) {
	proiezione := forma("cartiglio", sel("cartiglio.codice"), pBase())
	proiezione.Completa = false
	proiezione.SegmentiMancanti = []string{"T"}
	proiezione.ConfineDopo = classe(grammatica.ConfineParolaASCIIOPuntoCifra)
	scenari := []struct {
		nome      string
		cartiglio formaEsempio
		casi      []casoLettura
	}{
		{"con la revisione [base][rev-barra]", formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase(), pRif(grammatica.TipoParteRevisione, "rev-barra", 1)), "9.123.4567.3/01"}, []casoLettura{
			{"corpo", "9.123.4567.3/01", "completa", "9.123.4567.3/01", ""},
			{"corpo", "9.123.4567.3/xx", "completa", "", MotivoComposizioneRevisioneAmbigua},
			{"oggetto", "9.123.4567.3_30", "completa-sottolineato", "", MotivoComposizioneRevisioneAmbigua},
			{"corpo", "9.123.4567", "parziale", "", MotivoComposizioneLetturaNonCompleta},
			{"corpo", "9.123.4567.3", "completa", "", MotivoComposizioneRevisioneNonDeterminata},
		}},
		{"solo la base [base]", formaEsempio{forma("cartiglio", sel("cartiglio.codice"), pBase()), "9.123.4567.3"}, []casoLettura{
			{"corpo", "9.123.4567.3", "completa", "9.123.4567.3", ""},
			{"oggetto", "S9.123.4567.3", "completa", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"corpo", "9.123.4567.3/01", "completa", "", MotivoComposizioneQualificatoreNonTrasferibile},
			{"corpo", "9.123.4567.3/xx", "completa", "", MotivoComposizioneRevisioneAmbigua},
			{"corpo", "9.123.4567", "parziale", "", MotivoComposizioneLetturaNonCompleta},
		}},
		{"il cartiglio è una proiezione senza T", formaEsempio{proiezione, "9.123.4567"}, []casoLettura{
			{"corpo", "9.123.4567.3", "completa", "", MotivoComposizioneParteNonDeterminata},
			{"corpo", "9.123.4567", "parziale", "", MotivoComposizioneLetturaNonCompleta},
		}},
	}
	for _, sc := range scenari {
		t.Run(sc.nome, func(t *testing.T) {
			m, _ := compilaBene(t, grammaticaACME(famPuntiCartiglio(sc.cartiglio)))
			controllaCasi(t, m, "acme-punti", "cartiglio", sc.casi)
		})
	}
}

// TestComponiNonInventaLeMaiuscole (A3.3; T9): la base si scrive normalizzata come la dichiara la famiglia, mai
// con una maiuscola scelta dal compositore. Con le maiuscole indifferenti e la normalizzazione in maiuscolo la
// stringa è maiuscola; senza normalizzazione resta com'è letta; se la forma normalizzata non sta nella forma del
// cartiglio (maiuscole esatte, pattern minuscolo), la rilettura lo vede e la stringa non c'è.
func TestComponiNonInventaLeMaiuscole(t *testing.T) {
	casi := []struct {
		nome                  string
		maiuscole, normalizza string
		pattern, esempio      string
		letto                 string
		atteso, motivo        string
	}{
		{"indifferenti, normalizzate", grammatica.MaiuscoleIndifferentiASCII, grammatica.NormalizzaMaiuscolo, "[A-Z]{2}[0-9]{4}", "AB1234", "ab1234", "AB1234", ""},
		{"indifferenti, miste", grammatica.MaiuscoleIndifferentiASCII, grammatica.NormalizzaMaiuscolo, "[A-Z]{2}[0-9]{4}", "AB1234", "aB1234", "AB1234", ""},
		{"indifferenti, non normalizzate", grammatica.MaiuscoleIndifferentiASCII, grammatica.NormalizzaNessuna, "[A-Z]{2}[0-9]{4}", "AB1234", "ab1234", "ab1234", ""},
		{"esatte, la normalizzata fuori dalla forma", grammatica.MaiuscoleEsatte, grammatica.NormalizzaMaiuscolo, "[a-z]{2}[0-9]{4}", "ab1234", "ab1234", "", MotivoComposizioneParteNonDeterminata},
	}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			m, _ := compilaBene(t, grammaticaACME(famMaiuscole(cs.maiuscole, cs.normalizza, cs.pattern, cs.esempio)))
			controllaCasi(t, m, "acme-maiuscole", "cartiglio", []casoLettura{{"nodo_step.id", cs.letto, "nodo", cs.atteso, cs.motivo}})
		})
	}
}

// TestComponiNonInventaSuLettureCostruite (A3.3): letture che Riconosci non darebbe, costruite cambiando una
// lettura vera. Il compositore non si fida dei campi: una lettura incoerente, o una parte che la forma del
// cartiglio non sa scrivere, non dà stringa; ciò che la grammatica dichiara fuori dal codice (etichetta,
// decorazioni) non conta. Anche un motore nullo e una famiglia che il motore non ha danno un motivo, mai un panico.
func TestComponiNonInventaSuLettureCostruite(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famKit(cartiglioStd())))
	buona := leggiUna(t, m, "nome_file", "8123456A_2.stp", famigliaKit, "nome")
	if got := componiControllato(t, m, buona); got.Testo != "8123456A2" {
		t.Fatalf("la lettura di partenza non compone: %+v", got)
	}
	parte := func(v string) *ParteLetta { return &ParteLetta{Valore: v, Originale: v} }
	casi := []struct {
		nome   string
		cambia func(l *LetturaForma)
		atteso string
		motivo string
	}{
		{"stato vuoto", func(l *LetturaForma) { l.Stato = "" }, "", MotivoComposizioneLetturaNonCompleta},
		{"stato parziale", func(l *LetturaForma) { l.Stato = StatoParziale }, "", MotivoComposizioneLetturaNonCompleta},
		{"stato discordante", func(l *LetturaForma) { l.Stato = StatoDiscordante }, "", MotivoComposizioneLetturaNonCompleta},
		{"base non completa", func(l *LetturaForma) { l.Base.Completa = false }, "", MotivoComposizioneLetturaNonCompleta},
		{"segmenti mancanti", func(l *LetturaForma) { l.Base.Mancanti = []string{"coda"} }, "", MotivoComposizioneLetturaNonCompleta},
		{"base vuota", func(l *LetturaForma) { l.Base = BaseLetta{Completa: true} }, "", MotivoComposizioneLetturaNonCompleta},
		{"namespace di un'altra famiglia", func(l *LetturaForma) { l.Namespace = "acme-altro" }, "", MotivoComposizioneLetturaNonCompleta},
		{"ripetizione discordante", func(l *LetturaForma) {
			l.Ripetizioni = []RipetizioneLetta{{Base: BaseLetta{Originale: "8123457", Normalizzata: "8123457", Completa: true}}}
		}, "", MotivoComposizioneLetturaNonCompleta},
		{"stato da verificare", func(l *LetturaForma) { l.Stato = StatoDaVerificare }, "", MotivoComposizioneRevisioneAmbigua},
		{"revisione ambigua", func(l *LetturaForma) { l.Revisione.Stato = "ambigua" }, "", MotivoComposizioneRevisioneAmbigua},
		{"revisione letta senza valore", func(l *LetturaForma) { l.Revisione.Normalizzata = "" }, "", MotivoComposizioneRevisioneAmbigua},
		{"revisione assente", func(l *LetturaForma) { l.Revisione = nil }, "", MotivoComposizioneRevisioneNonDeterminata},
		{"revisione di un'altra regola", func(l *LetturaForma) { l.Revisione.Regola = "rev-altra" }, "", MotivoComposizioneParteNonDeterminata},
		{"revisione con uno zero in più", func(l *LetturaForma) { l.Revisione.Normalizzata = "02" }, "", MotivoComposizioneParteNonDeterminata},
		{"marcatore minuscolo", func(l *LetturaForma) { l.Marcatore = parte("a") }, "", MotivoComposizioneParteNonDeterminata},
		{"marcatore assente", func(l *LetturaForma) { l.Marcatore = nil }, "", MotivoComposizioneParteNonDeterminata},
		{"base che non concorda con i segmenti", func(l *LetturaForma) { l.Base.Normalizzata = "8123457" }, "", MotivoComposizioneParteNonDeterminata},
		{"base fuori dal pattern", func(l *LetturaForma) {
			l.Base.Normalizzata = "ABC"
			l.Base.Segmenti = []SegmentoLetto{{Nome: "numero", Originale: "ABC", Normalizzato: "ABC"}}
		}, "", MotivoComposizioneParteNonDeterminata},
		{"affisso non attribuito senza posto", func(l *LetturaForma) {
			l.Affissi = append(l.Affissi, AffissoLetto{Regola: "S", Originale: "S", Posizione: grammatica.PosizionePrefisso})
		}, "", MotivoComposizioneQualificatoreNonTrasferibile},
		{"token senza posto", func(l *LetturaForma) { l.Token = parte("PRT") }, "", MotivoComposizioneQualificatoreNonTrasferibile},
		{"etichetta letta", func(l *LetturaForma) { l.Etichetta = parte("PN") }, "8123456A2", ""},
		{"decorazioni lette", func(l *LetturaForma) {
			l.Decorazioni = append(l.Decorazioni, DecorazioneLetta{Regola: "pdm", Tipo: grammatica.TipoDecorazioneStatoPDM, Valore: "IN_WORK", Originale: "IN_WORK"})
		}, "8123456A2", ""},
		{"selettore e forma che il motore non ha", func(l *LetturaForma) { l.Forma = "inesistente" }, "8123456A2", ""},
	}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			l := copiaLettura(buona)
			cs.cambia(&l)
			got := componiControllato(t, m, l)
			want := CodiceComposto{Testo: cs.atteso, Famiglia: famigliaKit, Forma: "cartiglio", Motivo: cs.motivo}
			if got != want {
				t.Errorf("%+v, atteso %+v", got, want)
			}
		})
	}

	t.Run("famiglia che il motore non ha", func(t *testing.T) {
		l := copiaLettura(buona)
		l.Famiglia = "acme-sconosciuta"
		want := CodiceComposto{Famiglia: "acme-sconosciuta", Motivo: MotivoComposizioneFormaAssente}
		if got := componiControllato(t, m, l); got != want {
			t.Errorf("%+v, atteso %+v", got, want)
		}
	})
	t.Run("forma riservata del cartiglio", func(t *testing.T) {
		mr, _ := compilaBene(t, grammaticaACME(famCampoSeparato(), famLavagna()))
		l := LetturaForma{Famiglia: "acme-campo-separato-lavagna", Namespace: "acme-campo-separato", Stato: StatoCompleta}
		want := CodiceComposto{Famiglia: "acme-campo-separato-lavagna", Motivo: MotivoComposizioneFormaAssente}
		if got := componiControllato(t, mr, l); got != want {
			t.Errorf("%+v, atteso %+v", got, want)
		}
	})
	t.Run("motore nullo", func(t *testing.T) {
		var nullo *Motore
		want := CodiceComposto{Famiglia: famigliaKit, Motivo: MotivoComposizioneFormaAssente}
		if got := nullo.ComponiCodiceDocumentale(buona); got != want {
			t.Errorf("%+v, atteso %+v", got, want)
		}
	})
}

// ---- determinismo e autosufficienza ----

// letturaDaComporre: una lettura delle prove di determinismo, con il risultato atteso.
type letturaDaComporre struct {
	sel, testo, famiglia, forma, atteso, motivo string
}

var lettureDET = []letturaDaComporre{
	{"nome_file", "9123456A_2.stp", "acme-marcatore", "nome-sottolineato", "9123456A2", ""},
	{"nodo_step.id", "9123456A", "acme-marcatore", "step", "", MotivoComposizioneRevisioneNonDeterminata},
	{"radice_step.id", "9123456A_PRT", "acme-marcatore", "step-prt", "", MotivoComposizioneRevisioneNonDeterminata},
	{"corpo", "P7120100", "acme-prefisso", "mail", "", MotivoComposizioneQualificatoreNonTrasferibile},
	{"cartiglio.codice", "7120100", "acme-prefisso", "cartiglio", "7120100", ""},
	{"nome_file", "T+300.012345.010.pdf", "acme-campo-separato", "nome", "T+300.012345.010", ""},
	{"nodo_step.id", "912345X1", "acme-marcatore-x", "x-nodo", "", MotivoComposizioneFormaAssente},
	{"corpo", "9.123.4567.3/01", "acme-punti", "completa", "", MotivoComposizioneFormaAssente},
	{"nome_file", "8123456A_2.stp", famigliaKit, "nome", "8123456A2", ""},
	{"nome_file", "ACME-0308123456A_2.stp", famigliaKit, "nome", "8123456A2", ""},
	{"storia", "8123456A_x", famigliaKit, "sospesa", "", MotivoComposizioneRevisioneAmbigua},
	{"corpo", "8123456#R8123457", famigliaKit, "ripetuta", "", MotivoComposizioneLetturaNonCompleta},
}

// TestComponiEDeterministico (contratto A1c §2.1; par.3.4; T-B0-17): tutte le grammatiche ACME in un motore solo,
// con la famiglia del compositore; famiglie, forme, letterali e selettori in un altro ordine, o uno snapshot non
// normalizzato, danno lo stesso CodiceComposto, e cento chiamate pure. Nessun ordine dipende da una mappa.
func TestComponiEDeterministico(t *testing.T) {
	g := grammaticaACME(append(tutteLeFamiglie(), famKit(cartiglioStd()))...)
	m1, _ := compilaBene(t, g)
	m2, _ := compilaBene(t, permutata(g))
	m3, d3, err := CompilaVerificato(grammatica.SnapshotRegole{ClienteID: clienteACME, Grammatica: permutata(g)}, limitiACME())
	if err != nil {
		t.Fatalf("snapshot non normalizzato: %v\n%s", err, elenco(d3))
	}
	for _, in := range lettureDET {
		l1 := leggiUna(t, m1, in.sel, in.testo, in.famiglia, in.forma)
		c1 := componiControllato(t, m1, l1)
		formaUsata := ""
		if in.motivo != MotivoComposizioneFormaAssente {
			formaUsata = "cartiglio"
		}
		want := CodiceComposto{Testo: in.atteso, Famiglia: in.famiglia, Forma: formaUsata, Motivo: in.motivo}
		if c1 != want {
			t.Errorf("%s %q: %+v, atteso %+v", in.sel, in.testo, c1, want)
		}
		for nome, m := range map[string]*Motore{"permutata": m2, "non normalizzata": m3} {
			l := leggiUna(t, m, in.sel, in.testo, in.famiglia, in.forma)
			if c := componiControllato(t, m, l); c != c1 {
				t.Errorf("%s %q, %s: %+v, la prima %+v", in.sel, in.testo, nome, c, c1)
			}
			if c := m.ComponiCodiceDocumentale(l1); c != c1 {
				t.Errorf("%s %q, %s, con la lettura del primo motore: %+v, la prima %+v", in.sel, in.testo, nome, c, c1)
			}
		}
		b1 := canonico(t, c1)
		for i := 0; i < 100; i++ {
			if c := m1.ComponiCodiceDocumentale(l1); c != c1 || string(canonico(t, c)) != string(b1) {
				t.Fatalf("%s %q: la chiamata %d è diversa dalla prima", in.sel, in.testo, i)
			}
		}
	}
}

// TestComponiNonDipendeDalloSnapshotDelChiamante (E4): le parti che il compositore legge le congela
// CompilaVerificato nel motore, copiate dalla grammatica normalizzata. Il chiamante cambia dopo la compilazione
// le slice del suo snapshot (il letterale del marcatore e la regola della revisione della forma del cartiglio, i
// separatori delle revisioni): la composizione non cambia.
func TestComponiNonDipendeDalloSnapshotDelChiamante(t *testing.T) {
	s, err := grammatica.NuovoSnapshot(fileJSON(t, grammaticaACME(famMarcatore())), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := CompilaVerificato(s, limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	l := leggiUna(t, m, "nome_file", "9123456A_2.stp", "acme-marcatore", "nome-sottolineato")
	prima := componiControllato(t, m, l)
	if prima.Testo != "9123456A2" {
		t.Fatalf("la lettura non compone: %+v", prima)
	}
	cambiate := 0
	for i := range s.Grammatica.Famiglie {
		f := &s.Grammatica.Famiglie[i]
		for j := range f.Forme {
			if f.Forme[j].ID != "cartiglio" {
				continue
			}
			for k := range f.Forme[j].Parti {
				pt := &f.Forme[j].Parti[k]
				if pt.Tipo == grammatica.TipoParteMarcatore {
					pt.Letterali[0] = "Z"
					cambiate++
				}
				if pt.Tipo == grammatica.TipoParteRevisione {
					pt.Rif = "altra"
					cambiate++
				}
			}
		}
		for j := range f.Revisioni {
			f.Revisioni[j].Separatori = []string{"-", "/"}
		}
	}
	if cambiate != 2 {
		t.Fatalf("la prova non cambia lo snapshot del chiamante (%d parti)", cambiate)
	}
	if dopo := componiControllato(t, m, l); dopo != prima {
		t.Fatalf("lo snapshot del chiamante cambiato dopo la compilazione cambia la composizione: prima %+v, dopo %+v", prima, dopo)
	}
}
