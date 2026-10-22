// L1 — le equivalenze dal nome del file (KN-01, decisione dell'utente del 22/10; equivalenze.go, relazioniDelNome): una
// forma del nome che la grammatica dichiara con stessa_parte_di lega, nello stesso nome, il documento al codice della
// parte, come il cartiglio di K4: il nodo dello STEP con quel documento mostra il codice della parte (proposto), un 2D
// letto solo come codice della parte è proposto sul nodo, due fonti della stessa coppia sono una coppia con due prove,
// una lettura che contraddice la coppia la toglie con l'avviso, un nome senza la forma dichiarata non lega niente, e
// senza il campo nella grammatica non cambia niente. Il motore non sa niente del cliente: le etichette, il prefisso
// facoltativo e quali famiglie sono il codice della parte stanno solo nella grammatica sintetica di questa prova.
package ancoraggio_test

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati. Tre spazi di codici ACME: il materiale «acme» (21 più sette cifre, con un 9
// facoltativo davanti nei soli nomi dei file), il documento 3D «acme-doc» (101 più sei cifre e il foglio, l'id dei nodi
// dello STEP) e il documento 2D «acme-doc2» (102 più sei cifre e il foglio). Le etichette inventate T3 e T2 dicono, nel
// nome, quale documento segue il materiale: «210000002_T3_101000002_01.stp», «210000002_T2_102000002_01.pdf».

func baseDocNome(prefisso string) grammatica.Base {
	return base(grammatica.SegmentoBase{Nome: "documento", Pattern: prefisso + "[0-9]{6}", Identitario: true},
		grammatica.SegmentoBase{Nome: "foglio", Pattern: "[0-9]{2}", Separatore: "_", Identitario: true})
}

// famMaterialeNome: il materiale, sugli id dello STEP e sul cartiglio; nel nome del file con il 9 facoltativo davanti
// (un'etichetta facoltativa, fuori dal codice).
func famMaterialeNome() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-mat", Namespace: "acme",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "21[0-9]{7}", Identitario: true}),
		Forme: []grammatica.FormaCodice{
			{ID: "codice", Selettori: []string{"cartiglio.codice", "radice_step.id", "nodo_step.id"}, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}},
			{ID: "nome", Selettori: []string{"nome_file"}, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteEtichetta, Rif: "pre", Min: 0, Max: 1}, {Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}},
		},
		Etichette: []grammatica.Etichetta{{ID: "pre", Letterali: []string{"9"}, SpaziMax: 0, Selettori: []string{"nome_file"}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-nodo", "nodo_step.id", "210000001", false, grammatica.LetturaAttesa{Forma: "codice", Base: "210000001"}),
			esempio("e-nome", "nome_file", "210000002_T3_101000002_01.stp", true, grammatica.LetturaAttesa{Forma: "nome", Base: "210000002"}),
			esempio("e-nome-pre", "nome_file", "9210000002_T3_101000002_01.stp", true, grammatica.LetturaAttesa{Forma: "nome", Base: "210000002"}),
		},
	}
}

// famDocumento3DNome: il documento 3D, sugli id dello STEP e sul riferimento 3D del cartiglio; nel nome dopo
// l'etichetta T3, con la relazione della stessa parte verso il materiale (stessa_parte_di), e senza etichetta con la
// forma «nudo», che non la dichiara.
func famDocumento3DNome(relazione bool) grammatica.FamigliaCodice {
	var spd []string
	if relazione {
		spd = []string{"acme-mat"}
	}
	pBase := []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}
	return grammatica.FamigliaCodice{
		ID: "acme-doc", Namespace: "acme-doc",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  baseDocNome("101"),
		Forme: []grammatica.FormaCodice{
			{ID: "step", Selettori: []string{"radice_step.id", "nodo_step.id"}, Stato: grammatica.StatoAttiva, Completa: true, Parti: pBase},
			{ID: "riferimento", Selettori: []string{"cartiglio.riferimento_3d"}, Stato: grammatica.StatoAttiva, Completa: true, Parti: pBase},
			{ID: "nome-t3", Selettori: []string{"nome_file"}, Stato: grammatica.StatoAttiva, Completa: true, StessaParteDi: spd,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteEtichetta, Rif: "t3", Min: 1, Max: 1},
					{Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_"}, Min: 1, Max: 1}, {Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}},
			{ID: "nudo", Selettori: []string{"nome_file"}, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}, {Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_"}, Min: 1, Max: 1},
					{Tipo: grammatica.TipoParteToken, Letterali: []string{"000"}, Rimando: "Q-ACME-NOME", Min: 1, Max: 1}}},
		},
		Etichette: []grammatica.Etichetta{{ID: "t3", Letterali: []string{"T3"}, SpaziMax: 0, Selettori: []string{"nome_file"}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-step", "radice_step.id", "101000001_01", false, grammatica.LetturaAttesa{Forma: "step", Base: "101000001_01"}),
			esempio("e-rif", "cartiglio.riferimento_3d", "101000002_01", false, grammatica.LetturaAttesa{Forma: "riferimento", Base: "101000002_01"}),
			esempio("e-nome", "nome_file", "210000002_T3_101000002_01.stp", true, grammatica.LetturaAttesa{Forma: "nome-t3", Base: "101000002_01"}),
			esempio("e-nudo", "nome_file", "210000002_101000002_01_000.stp", true, grammatica.LetturaAttesa{Forma: "nudo", Base: "101000002_01"}),
		},
	}
}

// famDocumento2DNome: il documento 2D, solo nel nome dopo l'etichetta T2, con la relazione verso il materiale.
func famDocumento2DNome() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-doc2", Namespace: "acme-doc2",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  baseDocNome("102"),
		Forme: []grammatica.FormaCodice{{ID: "nome-t2", Selettori: []string{"nome_file"}, Stato: grammatica.StatoAttiva, Completa: true,
			StessaParteDi: []string{"acme-mat"},
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteEtichetta, Rif: "t2", Min: 1, Max: 1},
				{Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_"}, Min: 1, Max: 1}, {Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Etichette: []grammatica.Etichetta{{ID: "t2", Letterali: []string{"T2"}, SpaziMax: 0, Selettori: []string{"nome_file"}, Obbligatoria: true}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-nome", "nome_file", "210000002_T2_102000002_01.pdf", true, grammatica.LetturaAttesa{Forma: "nome-t2", Base: "102000002_01"}),
		},
	}
}

func motoreNome(t *testing.T, relazione bool) *motorea.Motore {
	t.Helper()
	return motore(t, famMaterialeNome(), famDocumento3DNome(relazione), famDocumento2DNome())
}

// scenaNome: l'assieme «210000001_T3_101000001_01.stp» (la radice #1 è il documento 101000001_01, il figlio #2 il
// documento 101000002_01) e il target 210000001.
func scenaNome(t *testing.T, m *motorea.Motore) (ancoraggio.FileInterpretato, ancoraggio.ContestoStrutturale, ancoraggio.ProdottoRichiesto) {
	t.Helper()
	step, s := stepRis(t, m, idStepR, "210000001_T3_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	return step, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}, targetRis(t, m, cProdottoR)
}

// stepPezzo: lo STEP di un pezzo solo, con la radice #1 dal documento dato e il nome dato.
func stepPezzo(t *testing.T, m *motorea.Motore, n int, nome, documento string) ancoraggio.FileInterpretato {
	t.Helper()
	f, _ := stepRis(t, m, uidS(n), nome, shaS(string("0123456789abcdef"[n%16]))[:62]+"0"+string("0123456789abcdef"[(n/16)%16]),
		[]string{"#1"}, []nodoS{{"#1", documento, "PEZZO"}}, nil, "")
	return f
}

// equivalenteDi: il codice della parte proposto sul nodo dell'assieme, con le fonti delle prove; nil senza.
func equivalenteDi(t *testing.T, e ancoraggio.EsitoAncoraggi, chiave string) (*ancoraggio.CodiceEquivalente, []string) {
	t.Helper()
	for _, s := range e.Strutture {
		for _, n := range s.Nodi {
			if n.Rif != ancoraggio.RifNodo(shaStepR, chiave) || n.Codice.Equivalente == nil {
				continue
			}
			var fonti []string
			for _, p := range n.Codice.Equivalente.Prove {
				fonti = append(fonti, p.Fonte)
			}
			return n.Codice.Equivalente, fonti
		}
	}
	return nil, nil
}

// TestLaRelazioneDelNomeLegaLaRadiceEIlFiglio: il nome dell'assieme lega la radice al target (la radice resta quella
// della struttura dal nome, con l'identità dello STEP), il nome dello STEP del pezzo lega il figlio #2 al suo materiale:
// i due nodi mostrano il codice della parte, proposto, con la fonte nome_file_relazione e la prova (l'allegato, l'unità
// del nome, le due letture). La lettura del documento porta le famiglie dichiarate; nessuna contesa.
func TestLaRelazioneDelNomeLegaLaRadiceEIlFiglio(t *testing.T) {
	m := motoreNome(t, true)
	step, ctx, tg := scenaNome(t, m)
	pezzo := stepPezzo(t, m, 0xb01, "210000002_T3_101000002_01.stp", "101000002_01")
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo}, []ancoraggio.ProdottoRichiesto{tg}, ctx)

	if len(e.Strutture) != 1 || e.Strutture[0].Origine != ancoraggio.OrigineStrutturaNomeFileSTEP {
		t.Fatalf("la struttura dal nome dell'assieme: %+v", e.Strutture)
	}
	eq, fonti := equivalenteDi(t, e, "#1")
	if eq == nil || eq.Namespace != "acme" || eq.Base != "210000001" || eq.Fonte() != ancoraggio.FonteEquivalenzaNomeFileRelazione ||
		!reflect.DeepEqual(fonti, []string{ancoraggio.FonteEquivalenzaNomeFileRelazione}) || eq.Prove[0].AllegatoID != step.AllegatoID ||
		eq.Prove[0].Entita == "" || eq.Disegni() != 1 {
		t.Errorf("la radice: il materiale dal nome dell'assieme %+v %v", eq, fonti)
	}
	var codice, documento, unita string
	for _, l := range step.Interpretazione.Letture {
		switch {
		case l.Forma.Forma == "nome" && l.Forma.Famiglia == "acme-mat":
			codice, unita = l.ID, l.UnitaID
			if l.Forma.StessaParteDi != nil {
				t.Errorf("la lettura del codice della parte non dichiara niente: %+v", l.Forma.StessaParteDi)
			}
		case l.Forma.Forma == "nome-t3":
			documento = l.ID
			if !reflect.DeepEqual(l.Forma.StessaParteDi, []string{"acme-mat"}) {
				t.Errorf("la lettura del documento porta le famiglie della grammatica: %+v", l.Forma.StessaParteDi)
			}
		}
	}
	if eq != nil && (eq.Prove[0].LetturaCodice != codice || eq.Prove[0].LetturaRiferimento != documento || eq.Prove[0].Entita != unita) {
		t.Errorf("la prova dal nome: %+v (codice %s, documento %s)", eq.Prove[0], codice, documento)
	}
	if eq, _ := equivalenteDi(t, e, "#2"); eq == nil || eq.Base != "210000002" || eq.Prove[0].AllegatoID != pezzo.AllegatoID {
		t.Errorf("il figlio #2: il materiale dal nome dello STEP del pezzo %+v", eq)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 0 {
		t.Errorf("nessuna contesa: %+v", d)
	}
}

// TestLaRelazioneDelNomeColPrefissoEIl2D: lo STEP del pezzo con il 9 davanti al materiale lega lo stesso codice (il
// prefisso è un'etichetta della grammatica, fuori dal codice); il 2D del pezzo, letto solo dal nome (materiale e
// documento 2D), è proposto sul nodo #2 per l'equivalenza, e la sua coppia con il documento 2D (un altro namespace)
// sta accanto senza contesa.
func TestLaRelazioneDelNomeColPrefissoEIl2D(t *testing.T) {
	m := motoreNome(t, true)
	step, ctx, tg := scenaNome(t, m)
	pezzo := stepPezzo(t, m, 0xb02, "9210000002_T3_101000002_01.stp", "101000002_01")
	disegno := pdfEq(t, m, 0xb03, "9210000002_T2_102000002_01.pdf")
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo, disegno}, []ancoraggio.ProdottoRichiesto{tg}, ctx)

	if eq, _ := equivalenteDi(t, e, "#2"); eq == nil || eq.Base != "210000002" || eq.Fonte() != ancoraggio.FonteEquivalenzaNomeFileRelazione {
		t.Errorf("il figlio #2 dal nome con il prefisso: %+v", eq)
	}
	c := candidatoEq(t, e, disegno.AllegatoID)
	if len(c.Equivalenze) != 1 || c.Equivalenze[0].AllegatoID != pezzo.AllegatoID || c.Equivalenze[0].Fonte != ancoraggio.FonteEquivalenzaNomeFileRelazione {
		t.Errorf("il 2D del pezzo, per l'equivalenza dal nome dello STEP: %+v", c.Equivalenze)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 0 {
		t.Errorf("il documento 2D e il documento 3D dello stesso codice non sono una contesa: %+v", d)
	}
}

// TestLaRelazioneDelNomeConIlCartiglio: il cartiglio di un 2D e il nome dello STEP del pezzo dicono la stessa coppia:
// una coppia sola con due prove, la fonte del codice è il cartiglio.
func TestLaRelazioneDelNomeConIlCartiglio(t *testing.T) {
	m := motoreNome(t, true)
	step, ctx, tg := scenaNome(t, m)
	pezzo := stepPezzo(t, m, 0xb04, "210000002_T3_101000002_01.stp", "101000002_01")
	disegno := pdfEq(t, m, 0xb05, "disegno.pdf", campoP{"codice", "210000002"}, campoP{"riferimento_3d", "101000002_01"})
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo, disegno}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	eq, fonti := equivalenteDi(t, e, "#2")
	if eq == nil || eq.Base != "210000002" || eq.Fonte() != ancoraggio.FonteEquivalenzaCartiglio || len(eq.Prove) != 2 || eq.Disegni() != 2 {
		t.Fatalf("una coppia con due prove: %+v", eq)
	}
	visti := map[string]bool{}
	for _, f := range fonti {
		visti[f] = true
	}
	if !visti[ancoraggio.FonteEquivalenzaCartiglio] || !visti[ancoraggio.FonteEquivalenzaNomeFileRelazione] {
		t.Errorf("le due prove, dal cartiglio e dal nome: %v", fonti)
	}
}

// TestLaRelazioneDelNomeContraddetta: il nome dello STEP del pezzo dice il documento 101000002_01 col materiale
// 210000002, il cartiglio di un 2D lo scrive accanto a 210000003: la coppia non si propone (nessun codice della parte sul
// nodo) e l'avviso dice la contesa. Un nome che dice due materiali accanto al documento è una contesa del nome.
func TestLaRelazioneDelNomeContraddetta(t *testing.T) {
	m := motoreNome(t, true)
	step, ctx, tg := scenaNome(t, m)
	pezzo := stepPezzo(t, m, 0xb06, "210000002_T3_101000002_01.stp", "101000002_01")
	disegno := pdfEq(t, m, 0xb07, "disegno.pdf", campoP{"codice", "210000003"}, campoP{"riferimento_3d", "101000002_01"})
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo, disegno}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if eq, _ := equivalenteDi(t, e, "#2"); eq != nil {
		t.Errorf("la coppia contraddetta non si propone: %+v", eq)
	}
	d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa)
	if len(d) == 0 || !contiene(d[0].Rif, "identita:acme-doc:101000002_01::") || !contiene(d[0].Rif, "identita:acme:210000003::") ||
		!contiene(d[0].Rif, "identita:acme:210000002::") || !strings.Contains(d[0].Messaggio, "nomi dei file") {
		t.Errorf("l'avviso della contesa, con le identità e il testo che dice anche i nomi dei file: %+v", d)
	}

	doppio := stepPezzo(t, m, 0xb08, "210000002_T3_101000002_01 210000003.stp", "101000002_01")
	e = ancoraA(t, []ancoraggio.FileInterpretato{step, doppio}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if eq, _ := equivalenteDi(t, e, "#2"); eq != nil {
		t.Errorf("il nome con due materiali non lega: %+v", eq)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 1 || !strings.Contains(d[0].Messaggio, "nome del file") {
		t.Errorf("la contesa del nome: %+v", d)
	}
}

// TestLaRelazioneDelNomeSoloConLaForma: un nome senza l'etichetta (il documento letto dalla forma «nudo», che non
// dichiara la relazione) non lega niente; e con la forma del 3D senza stessa_parte_di nemmeno il nome con l'etichetta
// T3. La famiglia del 2D lo dichiara ancora: la seconda metà prova anche che la coppia del 2D (codice e documento 2D,
// un altro namespace) non finisce mai sui nodi dello STEP, che hanno l'identità del documento 3D. I nodi non hanno il
// codice della parte, il 2D letto solo come materiale resta non determinabile, nessuna diagnostica nuova.
func TestLaRelazioneDelNomeSoloConLaForma(t *testing.T) {
	m := motoreNome(t, true)
	step, ctx, tg := scenaNome(t, m)
	pezzo := stepPezzo(t, m, 0xb09, "210000002_101000002_01_000.stp", "101000002_01")
	if !letto(pezzo, "nudo", "101000002_01") {
		t.Fatalf("il nome senza l'etichetta legge il documento con la forma che non dichiara la relazione: %+v", pezzo.Interpretazione.Letture)
	}
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if eq, _ := equivalenteDi(t, e, "#2"); eq != nil {
		t.Errorf("il nome senza l'etichetta non lega: %+v", eq)
	}

	senza := motoreNome(t, false)
	step, ctx, tg = scenaNome(t, senza)
	pezzo = stepPezzo(t, senza, 0xb0a, "210000002_T3_101000002_01.stp", "101000002_01")
	disegno := pdfEq(t, senza, 0xb0b, "210000002_T2_102000002_01.pdf")
	e = ancoraA(t, []ancoraggio.FileInterpretato{step, pezzo, disegno}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	for _, f := range []ancoraggio.FileInterpretato{step, pezzo} {
		for _, l := range f.Interpretazione.Letture {
			if l.Forma.Famiglia == "acme-doc" && l.Forma.StessaParteDi != nil {
				t.Errorf("senza il campo nella forma la lettura non dichiara niente: %+v", l.Forma)
			}
		}
	}
	if got := nodiConEquivalente(e); len(got) != 0 {
		t.Errorf("senza stessa_parte_di sulla forma del 3D nessun nodo ha il codice della parte, e la coppia del 2D non tocca il namespace del 3D: %v", got)
	}
	if got := collocazioneEq(t, e, disegno.AllegatoID); got != "non_determinabile [namespace_non_confrontabile]" {
		t.Errorf("senza la relazione del 3D, il 2D del pezzo: %s", got)
	}
	if d := conCodice(e.Diagnostiche, ancoraggio.CodiceEquivalenzaCartiglioContesa); len(d) != 0 {
		t.Errorf("nessuna contesa: %+v", d)
	}
}

// letto: il file ha una lettura della forma con la base data.
func letto(f ancoraggio.FileInterpretato, forma, base string) bool {
	for _, l := range f.Interpretazione.Letture {
		if l.Forma.Forma == forma && l.Forma.Base.Normalizzata == base {
			return true
		}
	}
	return false
}
