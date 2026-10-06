// L1 — le strutture dei prodotti negli STEP (piano A, 6.4.5, regole 1 e 2; contratto §0 punto 1, §1.2, §1.3, §2.2,
// §2.5, §7 famiglia B2): StrutturaDa dal documento dell'adattatore STEP e dalla sua interpretazione, poi
// ProponiStrutture. La struttura candidata prima del gesto e niente promosso (R59 A, PO-05); la BOM di lavoro solo
// sotto la radice scelta dello STEP confermato, nello stesso calcolo (R76 A, R85); i nodi per sha256 e chiave, mai per
// codice (T-E1-04); il prodotto come nodo interno (R76 b A); il contesto con l'origine e la riga della radice esclusa
// (R29 e, R61 A, R80); A1c-L1-07, -08, -09 e -20 nelle parti delle strutture; il determinismo.
package ancoraggio_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), i codici sono di fantasia (712xxxx, con la P di
// fase prototipo nella mail), i nomi dei file hanno il prefisso di progetto inventato «ACME-030P» e lo stato PDM
// generico «IN_WORK», gli UUID sono 00000000-0000-4000-8000-0000000000nn. La scena delle prove A1c-L1-07…09 è derivata
// dallo scenario sintetico delle prove del giro 4 (assiemi con figli in comune, uno STEP per pezzo), con nomi senza
// la sigla di nessun cliente. Le prove citano i requisiti (A1c-L1-07, R59 A, R76 A, T-E1-04, PO-05), mai i casi degli
// attesi.

// ---- la grammatica ACME delle strutture ----

var selStrutture = []string{"nome_file", "cartiglio.codice", "radice_step.id", "radice_step.nome", "nodo_step.id", "nodo_step.nome"}

// famStrutture: base 712 più quattro cifre. Nella mail con la P di fase prototipo davanti (D2), nei file e nello STEP
// senza la P: la stessa base, nello stesso namespace. Ruoli {prodotto, componente}.
func famStrutture() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-strutture", Namespace: "acme-strutture",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{
			{ID: "mail", Selettori: selMail, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{
				{Tipo: grammatica.TipoParteAffisso, Rif: "P", Min: 1, Max: 1}, {Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}},
			{ID: "codice", Selettori: selStrutture, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{
				{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}},
		},
		Affissi: []grammatica.Affisso{{
			ID: "P", Letterali: []string{"P"}, Posizione: grammatica.PosizionePrefisso,
			Riconoscimento: selMail, Attribuzione: selMail,
			Valore: &grammatica.ValoreQualificatore{Fase: grammatica.FasePrototipo},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-mail", "corpo", "P7120100", false, grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Affissi: []string{"P"}}),
			esempio("e-step", "radice_step.id", "7120100", false, grammatica.LetturaAttesa{Forma: "codice", Base: "7120100"}),
		},
	}
}

func motoreStrutture(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famStrutture())
}

// ---- gli STEP ACME, dai fatti del worker all'adattatore e a Interpreta ----

var (
	dataStrutture   = time.Date(2026, 10, 1, 7, 30, 0, 0, time.UTC)
	messaggioStep   = uuid.MustParse("00000000-0000-4000-8000-000000000501")
	ternaStrutture  = fotorfq.Terna{Versione: 4, HashConfigurazione: strings.Repeat("c", 64)}
	motivoSoloParti = "nessuna occorrenza di assieme: solo parti"
)

// uidS: un UUID di prova dal numero (nella forma 00000000-0000-4000-8000-0000000000nn).
func uidS(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", n)) }

// shaS: uno sha256 di prova, ripetendo una cifra esadecimale.
func shaS(c string) string { return strings.Repeat(c, 64) }

func testoS(s string) *string { return &s }

// nodoS: un PRODUCT dei fatti: la chiave, l'id e il nome grezzi.
type nodoS struct{ chiave, id, nome string }

// arcoS: una relazione dei fatti: padre, figlio, la quantità della coppia e i riferimenti NAUO delle occorrenze.
type arcoS struct {
	padre, figlio string
	qta           int
	righe         []string
}

// fattiS: i fatti di uno STEP alla terna corrente (struttura v3, la forma del worker). motivo è il motivo di
// completezza che il caricatore legge con struttura_motivo_parziale: "" completo, nil non determinabile.
func fattiS(t *testing.T, sha string, radici []string, nodi []nodoS, archi []arcoS, motivo *string) *fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": x.nome, "descrizione_grezza": "",
			"rev_grezza": "", "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		ev := map[string]any{}
		if len(a.righe) > 0 {
			ev["righe"] = a.righe
		}
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": ev})
	}
	if radici == nil {
		radici = []string{}
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": radici, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fattiGrezzi(t, sha, raw, motivo)
}

func fattiGrezzi(t *testing.T, sha string, raw json.RawMessage, motivo *string) *fotorfq.Fatti {
	t.Helper()
	f := fotorfq.Fatti{Sha256: sha, Terna: ternaStrutture, CalcolatoIl: dataStrutture, Payload: raw, MotivoParziale: motivo}
	d, err := fotorfq.ImprontaPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	f.Digest = d
	return &f
}

// fileS: l'allegato con i suoi fatti, passato dall'adattatore (estrazione.DaAllegato) e da Interpreta con l'uso
// sconosciuto, come lo prepara valutazione.
func fileS(t *testing.T, m *motorea.Motore, id uuid.UUID, nome, est, sha string, f *fotorfq.Fatti) ancoraggio.FileInterpretato {
	t.Helper()
	a := fotorfq.Allegato{ID: id, MessaggioID: messaggioStep, Indice: 1, NomeFile: nome, Estensione: testoS(est), Natura: "file",
		Origine: "outlook", Stato: "analizzato", Sha256: testoS(sha), RicevutoIl: dataStrutture}
	d, err := estrazione.DaAllegato(a, f, nil)
	if err != nil {
		t.Fatalf("DaAllegato %s: %v", nome, err)
	}
	r, err := m.Interpreta(d, evidenze.UsoSconosciuto(d.BundleID))
	if err != nil {
		t.Fatalf("Interpreta %s: %v", nome, err)
	}
	return ancoraggio.FileInterpretato{AllegatoID: id, Documento: d, Interpretazione: r}
}

// strutturaS: la struttura di uno STEP, che deve esserci.
func strutturaS(t *testing.T, f ancoraggio.FileInterpretato) ancoraggio.StrutturaFile {
	t.Helper()
	s, ok := ancoraggio.StrutturaDa(f)
	if !ok {
		t.Fatalf("lo STEP %s non dà una struttura", f.AllegatoID)
	}
	return s
}

// stepS: uno STEP d'assieme o di un pezzo, con la radice #1 e i figli dati; il nome del file è quello del codice
// della radice («ACME-030P<codice> 00 IN_WORK.stp»).
func stepS(t *testing.T, m *motorea.Motore, id uuid.UUID, sha string, nodi []nodoS, archi []arcoS, motivo string) ancoraggio.StrutturaFile {
	t.Helper()
	return strutturaS(t, fileS(t, m, id, "ACME-030P"+nodi[0].id+" 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{nodi[0].chiave}, nodi, archi, &motivo)))
}

// targetS: un prodotto target letto con la grammatica ACME: con la P (la mail dello scenario) o senza (un codice
// registrato). Un codice che la grammatica non legge dà la base vuota.
func targetS(t *testing.T, m *motorea.Motore, rif string, autorita ancoraggio.Autorita, codice string) ancoraggio.ProdottoRichiesto {
	t.Helper()
	p := ancoraggio.ProdottoRichiesto{Rif: rif, Autorita: autorita, ClienteID: clienteACME, CodiceRichiesto: codice}
	s := "nodo_step.id"
	if strings.HasPrefix(codice, "P") {
		s = "corpo"
	}
	sel, err := evidenze.LeggiSelettore(s)
	if err != nil {
		t.Fatal(err)
	}
	letture, _ := m.Riconosci(sel, codice)
	for _, l := range letture {
		if l.Intervallo.Inizio == 0 && l.Intervallo.Fine == len(codice) && l.Base.Completa {
			p.Namespace, p.Base, p.CodiceRichiesto = l.Namespace, l.Base, l.CodiceRichiesto
			break
		}
	}
	return p
}

// ---- le chiamate, con gli invarianti di ogni esito ----

// proponiS chiama ProponiStrutture e controlla gli invarianti di ogni esito:
//   - gli ingressi di chi chiama non cambiano;
//   - ogni struttura è candidata o BOM di lavoro proposta, mai altro (niente «verificata»: R80); al più una BOM per
//     target, e solo sotto la radice della sua fonte confermata, non superata (R76 A);
//   - una struttura candidata ha la radice compatibile con la base del target (regola 1);
//   - la radice è il primo nodo e non ha padri dentro la struttura; ogni altro nodo ha un arco entrante; i padri di
//     un nodo sono i padri dei suoi archi; ogni nodo è del file della struttura, con il Rif sha256 più chiave
//     (T-E1-04), e dice SenzaFile (nessun file si ancora qui);
//   - gli archi del contesto sono confermati o proposti, mai fatti;
//   - le fonti ci sono solo per una radice del file compatibile, fuori dallo STEP della fonte confermata (R76 b A);
//   - le diagnostiche sono quelle delle strutture, in avviso.
func proponiS(t *testing.T, target []ancoraggio.ProdottoRichiesto, ctx ancoraggio.ContestoStrutturale) ([]ancoraggio.StrutturaProdotto, []evidenze.Diagnostica) {
	t.Helper()
	primaT, primaC := canonico(t, target), canonico(t, ctx)
	strutture, diag, err := ancoraggio.ProponiStrutture(target, ctx)
	if err != nil {
		t.Fatalf("ProponiStrutture: %v", err)
	}
	if canonico(t, target) != primaT || canonico(t, ctx) != primaC {
		t.Error("ProponiStrutture ha cambiato gli ingressi di chi chiama")
	}
	perTarget := map[string]ancoraggio.ProdottoRichiesto{}
	for _, x := range target {
		perTarget[x.Rif] = x
	}
	bom := map[string]int{}
	for _, s := range strutture {
		tg, ok := perTarget[s.Target]
		if !ok {
			t.Errorf("struttura di un target che non c'è: %s", s.Target)
			continue
		}
		switch s.Stato {
		case ancoraggio.StatoStrutturaCandidata:
			if !compatibileS(s.Compatibilita) {
				t.Errorf("struttura candidata %s/%s con la radice %s non compatibile (%s): la regola 1 non la dà", s.Target, s.AllegatoID, s.Radice, s.Compatibilita)
			}
		case ancoraggio.StatoBOMDiLavoroProposta:
			bom[s.Target]++
			f := tg.FonteConfermata
			if f == nil || f.Superato || s.Sha256 != f.Sha256 || s.Radice != ancoraggio.RifNodo(f.Sha256, f.Radice) {
				t.Errorf("BOM di lavoro %s/%s fuori dalla radice scelta dello STEP confermato: %+v", s.Target, s.Radice, f)
			}
		default:
			t.Errorf("struttura %s/%s con lo stato %q", s.Target, s.Radice, s.Stato)
		}
		if len(s.Nodi) == 0 || s.Nodi[0].Rif != s.Radice || len(s.Nodi[0].Padri) != 0 {
			t.Errorf("struttura %s/%s: la radice non è il primo nodo senza padri: %+v", s.Target, s.Radice, s.Nodi)
		}
		nodi := map[string]bool{}
		for _, n := range s.Nodi {
			nodi[n.Rif] = true
			if !strings.HasPrefix(n.Rif, ancoraggio.PrefissoRifNodo+s.Sha256+":") || n.AllegatoID != s.AllegatoID || !n.SenzaFile {
				t.Errorf("struttura %s/%s: nodo %+v", s.Target, s.Radice, n)
			}
		}
		padri := map[string][]string{}
		for _, a := range s.Archi {
			if !nodi[a.Padre] || !nodi[a.Figlio] {
				t.Errorf("struttura %s/%s: arco fuori dai nodi %+v", s.Target, s.Radice, a)
			}
			padri[a.Figlio] = append(padri[a.Figlio], a.Padre)
		}
		for _, n := range s.Nodi[1:] {
			if len(padri[n.Rif]) == 0 || strings.Join(padri[n.Rif], ",") != strings.Join(n.Padri, ",") {
				t.Errorf("struttura %s/%s: il nodo %s ha i padri %v, gli archi dicono %v", s.Target, s.Radice, n.Rif, n.Padri, padri[n.Rif])
			}
		}
		for _, a := range s.ArchiContesto {
			if a.Origine != ancoraggio.OrigineArcoConfermato && a.Origine != ancoraggio.OrigineArcoProposto {
				t.Errorf("struttura %s/%s: arco del contesto con l'origine %q", s.Target, s.Radice, a.Origine)
			}
		}
		fonteQui := tg.FonteConfermata != nil && tg.FonteConfermata.Sha256 == s.Sha256
		if attese := s.RadiceDelFile && compatibileS(s.Compatibilita) && !fonteQui; attese != (len(s.Fonti) == 1) || len(s.Fonti) > 1 {
			t.Errorf("struttura %s/%s: fonti %+v (radice del file %v, compatibilità %s, STEP della fonte confermata %v)", s.Target, s.Radice, s.Fonti, s.RadiceDelFile, s.Compatibilita, fonteQui)
		}
	}
	for k, n := range bom {
		if n > 1 {
			t.Errorf("il target %s ha %d BOM di lavoro: al più una (R76 A)", k, n)
		}
	}
	for _, d := range diag {
		if (d.Codice != ancoraggio.CodiceTargetSenzaStruttura && d.Codice != ancoraggio.CodiceGrafoIncompleto) || d.Gravita != evidenze.GravitaAvviso || d.Natura != evidenze.NaturaDati {
			t.Errorf("diagnostica fuori dalle strutture: %+v", d)
		}
	}
	return strutture, diag
}

func compatibileS(c motorea.Compatibilita) bool {
	return c == motorea.CompatibilitaUguale || c == motorea.CompatibilitaEquivalente || c == motorea.CompatibilitaParziale
}

func canonico(t *testing.T, v any) string {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatalf("canonico: %v", err)
	}
	return string(b)
}

// struttureDi: le strutture di un target, nell'ordine dell'esito.
func struttureDi(s []ancoraggio.StrutturaProdotto, target string) []ancoraggio.StrutturaProdotto {
	var out []ancoraggio.StrutturaProdotto
	for _, x := range s {
		if x.Target == target {
			out = append(out, x)
		}
	}
	return out
}

// nodoDi: il nodo di una struttura con quel Rif.
func nodoDi(s ancoraggio.StrutturaProdotto, rif string) (ancoraggio.NodoProposto, bool) {
	for _, n := range s.Nodi {
		if n.Rif == rif {
			return n, true
		}
	}
	return ancoraggio.NodoProposto{}, false
}

func rifNodi(s ancoraggio.StrutturaProdotto) []string {
	var out []string
	for _, n := range s.Nodi {
		out = append(out, n.Rif)
	}
	return out
}

// ---- la scena ACME (A1c-L1-07…09, parti delle strutture) ----

// Gli allegati della scena: tre STEP d'assieme (due chiesti, uno no), due STEP di un pezzo («solo parti»), un PDF.
var (
	idAssieme100 = uidS(0x511)
	idAssieme103 = uidS(0x512)
	idAssieme102 = uidS(0x513)
	idPezzo110   = uidS(0x514)
	idPezzo112   = uidS(0x515)
)

var (
	shaAssieme100 = shaS("1")
	shaAssieme103 = shaS("2")
	shaAssieme102 = shaS("3")
	shaPezzo110   = shaS("4")
	shaPezzo112   = shaS("5")
)

// scenaACME: le strutture dei file della scena. L'assieme 7120100 ha quattro figli, fra cui una saldatura che
// nessuna famiglia legge e il pezzo 7120112 due volte (qta 2, due occorrenze); l'assieme 7120103 ha tre figli, fra
// cui lo stesso pezzo 7120112 (un figlio in comune fra due assiemi, in due file); l'assieme 7120102 nessuno lo chiede.
// I pezzi 7120110 e 7120112 hanno il loro STEP, «solo parti».
func scenaACME(t *testing.T, m *motorea.Motore, motivo100 string) []ancoraggio.StrutturaFile {
	t.Helper()
	return []ancoraggio.StrutturaFile{
		stepS(t, m, idAssieme100, shaAssieme100,
			[]nodoS{{"#1", "7120100", "TELAIO ACME"}, {"#2", "7120110", "PIASTRA"}, {"#3", "7120111", "LAMIERA"}, {"#4", "7120112", "TONDO"}, {"#5", "SALDATURA_1", ""}},
			[]arcoS{{"#1", "#2", 1, []string{"#21"}}, {"#1", "#3", 1, []string{"#22"}}, {"#1", "#4", 2, []string{"#23", "#24"}}, {"#1", "#5", 1, nil}}, motivo100),
		stepS(t, m, idAssieme103, shaAssieme103,
			[]nodoS{{"#1", "7120103", "STAFFA ACME"}, {"#2", "7120122", "SQUADRA"}, {"#3", "7120133", "VITE"}, {"#4", "7120112", "TONDO"}},
			[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 2, []string{"#31", "#32"}}, {"#1", "#4", 1, nil}}, ""),
		stepS(t, m, idAssieme102, shaAssieme102,
			[]nodoS{{"#1", "7120102", "SUPPORTO ACME"}, {"#2", "7120120", "PIASTRA"}, {"#3", "7120121", "BOCCOLA"}},
			[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}}, ""),
		stepS(t, m, idPezzo110, shaPezzo110, []nodoS{{"#1", "7120110", "PIASTRA"}}, nil, motivoSoloParti),
		stepS(t, m, idPezzo112, shaPezzo112, []nodoS{{"#1", "7120112", "TONDO"}}, nil, motivoSoloParti),
	}
}

// targetScenaACME: i due assiemi chiesti dalla mail, con l'autorità dello scenario (R75 A).
func targetScenaACME(t *testing.T, m *motorea.Motore) []ancoraggio.ProdottoRichiesto {
	t.Helper()
	return []ancoraggio.ProdottoRichiesto{
		targetS(t, m, "scenario:caso-acme:1", ancoraggio.AutoritaScenario, "P7120100"),
		targetS(t, m, "scenario:caso-acme:2", ancoraggio.AutoritaScenario, "P7120103"),
	}
}

// ---- la struttura del file ----

// TestStrutturaDelFile (6.4.5, StrutturaDa; T-E1-04; A1c-L1-09, la parte del file): dall'adattatore STEP alla
// struttura del file: le radici, i nodi con le letture d'identità, gli archi dei fatti con quantità e occorrenze, un
// figlio condiviso sotto due padri con i due archi, un nodo che nessuna famiglia legge, la completezza del grafo con
// il motivo; il Rif dei nodi è sha256 più chiave anche quando due file hanno lo stesso codice, e lo stesso contenuto
// in due allegati dà due strutture con gli stessi Rif.
func TestStrutturaDelFile(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("a")
	nodi := []nodoS{{"#1", "7120200", "GRUPPO ACME"}, {"#2", "7120210", "LATO DX"}, {"#3", "7120220", "LATO SX"}, {"#4", "7120230", "PERNO"},
		{"#5", "SALDATURA_1", "SALDATURA_1"}}
	archi := []arcoS{{"#1", "#2", 1, []string{"#11"}}, {"#1", "#3", 1, []string{"#12"}}, {"#2", "#4", 2, []string{"#13", "#14"}},
		{"#3", "#4", 1, []string{"#15"}}, {"#1", "#5", 1, nil}}
	f := fileS(t, m, uidS(0x520), "ACME-030P7120200 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{"#1"}, nodi, archi, testoS("")))
	s := strutturaS(t, f)

	r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
	if s.AllegatoID != f.AllegatoID || s.BundleID != f.Documento.BundleID || s.Sha256 != sha {
		t.Errorf("identità della struttura: %+v", s)
	}
	if !reflect.DeepEqual(s.Radici, []string{r("#1")}) {
		t.Errorf("radici %v, attesa solo la #1", s.Radici)
	}
	var rif []string
	for _, n := range s.Nodi {
		rif = append(rif, n.Rif)
		if n.Rif != ancoraggio.RifNodo(sha, n.Chiave) || n.EntitaID != "e:step:"+n.Chiave {
			t.Errorf("nodo %+v: il Rif è sha256 più chiave, l'entità quella dell'adattatore", n)
		}
	}
	if !reflect.DeepEqual(rif, []string{r("#1"), r("#2"), r("#3"), r("#4"), r("#5")}) {
		t.Errorf("nodi %v", rif)
	}
	for _, n := range s.Nodi {
		switch n.Chiave {
		case "#5":
			if len(n.Letture) != 0 {
				t.Errorf("la saldatura non la legge nessuna famiglia (D10): letture %+v", n.Letture)
			}
		default:
			if len(n.Letture) != 1 || n.Letture[0].Forma.Namespace != "acme-strutture" || n.Letture[0].Forma.Base.Normalizzata != map[string]string{
				"#1": "7120200", "#2": "7120210", "#3": "7120220", "#4": "7120230"}[n.Chiave] {
				t.Errorf("nodo %s: letture d'identità %+v", n.Chiave, n.Letture)
			}
			if want := map[bool]motorea.Funzione{true: motorea.FunzIdentitaFile, false: motorea.FunzStruttura}[n.Chiave == "#1"]; len(n.Letture) == 1 && n.Letture[0].Funzione != want {
				t.Errorf("nodo %s: funzione %q, attesa %q (router-2, righe 9 e 10)", n.Chiave, n.Letture[0].Funzione, want)
			}
		}
	}
	due, uno := 2, 1
	attesi := []ancoraggio.ArcoPercorso{
		{Padre: r("#1"), Figlio: r("#2"), Quantita: &uno, Occorrenze: []string{"#11"}, Origine: ancoraggio.OrigineArcoFatti},
		{Padre: r("#1"), Figlio: r("#3"), Quantita: &uno, Occorrenze: []string{"#12"}, Origine: ancoraggio.OrigineArcoFatti},
		{Padre: r("#1"), Figlio: r("#5"), Quantita: &uno, Origine: ancoraggio.OrigineArcoFatti},
		{Padre: r("#2"), Figlio: r("#4"), Quantita: &due, Occorrenze: []string{"#13", "#14"}, Origine: ancoraggio.OrigineArcoFatti},
		{Padre: r("#3"), Figlio: r("#4"), Quantita: &uno, Occorrenze: []string{"#15"}, Origine: ancoraggio.OrigineArcoFatti},
	}
	if !reflect.DeepEqual(s.Archi, attesi) {
		t.Errorf("archi:\n%+v\nattesi:\n%+v", s.Archi, attesi)
	}
	if !s.GrafoCompleto || s.MotivoGrafo != "" {
		t.Errorf("grafo completo atteso: %v %q", s.GrafoCompleto, s.MotivoGrafo)
	}

	t.Run("due file con lo stesso codice: due nodi (T-E1-04)", func(t *testing.T) {
		altro := shaS("b")
		g := strutturaS(t, fileS(t, m, uidS(0x521), "ACME-030P7120200 01 IN_WORK.stp", "stp", altro, fattiS(t, altro, []string{"#1"}, nodi[:1], nil, testoS(motivoSoloParti))))
		if g.Nodi[0].Rif == s.Nodi[0].Rif || g.Nodi[0].Rif != ancoraggio.RifNodo(altro, "#1") {
			t.Errorf("lo stesso codice in due file dà lo stesso nodo: %s e %s", g.Nodi[0].Rif, s.Nodi[0].Rif)
		}
	})
	t.Run("lo stesso contenuto in due allegati: due strutture, gli stessi Rif", func(t *testing.T) {
		g := strutturaS(t, fileS(t, m, uidS(0x522), "copia.stp", "stp", sha, fattiS(t, sha, []string{"#1"}, nodi, archi, testoS(""))))
		if g.AllegatoID == s.AllegatoID {
			t.Fatal("due allegati con lo stesso AllegatoID")
		}
		g.AllegatoID, g.BundleID = s.AllegatoID, s.BundleID
		if canonico(t, g) != canonico(t, s) {
			t.Errorf("lo stesso contenuto dà una struttura diversa:\n%s\n%s", canonico(t, g), canonico(t, s))
		}
	})
	t.Run("grafo incompleto, con il motivo", func(t *testing.T) {
		g := strutturaS(t, fileS(t, m, uidS(0x523), "troncato.stp", "stp", shaS("c"), fattiS(t, shaS("c"), []string{"#1"}, nodi, archi, testoS("lettura troncata: nodi"))))
		if g.GrafoCompleto || g.MotivoGrafo != "lettura troncata: nodi" {
			t.Errorf("grafo incompleto: %v %q", g.GrafoCompleto, g.MotivoGrafo)
		}
		n := strutturaS(t, fileS(t, m, uidS(0x524), "senza-motivo.stp", "stp", shaS("d"), fattiS(t, shaS("d"), []string{"#1"}, nodi, archi, nil)))
		if n.GrafoCompleto || n.MotivoGrafo == "" {
			t.Errorf("senza il motivo del caricatore la completezza non si dichiara: %v %q", n.GrafoCompleto, n.MotivoGrafo)
		}
	})
	t.Run("due radici", func(t *testing.T) {
		dueRadici := append(append([]nodoS(nil), nodi...), nodoS{"#9", "7120290", "ALTRO"})
		g := strutturaS(t, fileS(t, m, uidS(0x525), "due-radici.stp", "stp", shaS("e"), fattiS(t, shaS("e"), []string{"#1", "#9"}, dueRadici, archi,
			testoS("2 radici nel file: una distinta ne ha una"))))
		if !reflect.DeepEqual(g.Radici, []string{ancoraggio.RifNodo(shaS("e"), "#1"), ancoraggio.RifNodo(shaS("e"), "#9")}) || g.GrafoCompleto {
			t.Errorf("due radici: %v, grafo completo %v", g.Radici, g.GrafoCompleto)
		}
	})
	t.Run("niente struttura senza uno STEP letto (T-E1-09)", func(t *testing.T) {
		illeggibile := fattiGrezzi(t, shaS("f"), json.RawMessage(`{"struttura": {"versione": 3, "schema": "", "radici": [], "avvisi": ["non e' un file STEP Part 21"], "nodi": [], "relazioni": [], `+
			`"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`), nil)
		altroBundle := f
		altroBundle.Interpretazione.BundleID = shaS("0")
		for nome, x := range map[string]ancoraggio.FileInterpretato{
			"un PDF senza fatti":                      fileS(t, m, uidS(0x526), "ACME-030P7120200.pdf", "pdf", shaS("9"), nil),
			"uno STEP senza fatti":                    fileS(t, m, uidS(0x527), "ACME-030P7120200.stp", "stp", shaS("8"), nil),
			"uno STEP che il worker non legge":        fileS(t, m, uidS(0x528), "rotto.stp", "stp", shaS("f"), illeggibile),
			"un IGS":                                  fileS(t, m, uidS(0x529), "ACME-030P7120200.igs", "igs", shaS("7"), nil),
			"l'interpretazione di un altro documento": altroBundle,
		} {
			if s, ok := ancoraggio.StrutturaDa(x); ok {
				t.Errorf("%s dà una struttura: %+v", nome, s)
			}
		}
	})
}

// TestStrutturaDaLeGuardie (6.4.5, StrutturaDa; T-E1-09, R68 A): un documento con i nodi nodo_step, che senza le
// guardie darebbe una struttura, non la dà quando la capacità «struttura» manca o non è disponibile, quando la qualità
// è «errore», quando il riferimento ai fatti non ha lo sha256 o non è di un file analizzato; senza nodi non c'è
// struttura nemmeno con la capacità disponibile. Ogni variante tocca una cosa sola: i nodi restano, quindi il caso
// cade sulla sua guardia e non su «nessun nodo».
func TestStrutturaDaLeGuardie(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("a")
	f := fileS(t, m, uidS(0x5b0), "ACME-030P7120200 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{"#1"},
		[]nodoS{{"#1", "7120200", "GRUPPO"}, {"#2", "7120210", "LATO DX"}}, []arcoS{{"#1", "#2", 1, nil}}, testoS("")))
	if _, ok := ancoraggio.StrutturaDa(f); !ok {
		t.Fatal("il documento di base deve dare una struttura")
	}
	nodiSTEP := 0
	for _, e := range f.Documento.Entita {
		if e.Tipo == "nodo_step" {
			nodiSTEP++
		}
	}
	if nodiSTEP != 2 {
		t.Fatalf("il documento di base ha %d nodi nodo_step, attesi 2", nodiSTEP)
	}
	conCapacita := func(stato string, togli bool) ancoraggio.FileInterpretato {
		x := f
		x.Documento.Qualita.Capacita = nil
		for _, c := range f.Documento.Qualita.Capacita {
			if c.Nome == "struttura" {
				if togli {
					continue
				}
				c.Stato = stato
			}
			x.Documento.Qualita.Capacita = append(x.Documento.Qualita.Capacita, c)
		}
		return x
	}
	conErrore := f
	conErrore.Documento.Qualita.Stato = "errore"
	senzaSha := f
	senzaSha.Documento.Fonte.RiferimentoFatti.Sha256 = ""
	senzaFatti := f
	senzaFatti.Documento.Fonte.RiferimentoFatti.Tipo = "nessuno"
	senzaNodi := f
	senzaNodi.Documento.Entita = nil
	for _, e := range f.Documento.Entita {
		if e.Tipo != "nodo_step" {
			senzaNodi.Documento.Entita = append(senzaNodi.Documento.Entita, e)
		}
	}
	for nome, x := range map[string]ancoraggio.FileInterpretato{
		"capacità struttura assente":            conCapacita("", true),
		"capacità struttura non disponibile":    conCapacita("non_disponibile", false),
		"qualità errore":                        conErrore,
		"sha256 vuoto":                          senzaSha,
		"riferimento ai fatti non di un file":   senzaFatti,
		"nessun nodo, con la capacità presente": senzaNodi,
	} {
		if s, ok := ancoraggio.StrutturaDa(x); ok {
			t.Errorf("%s: una struttura %+v", nome, s)
		}
	}
	if _, ok := ancoraggio.StrutturaDa(conCapacita("parziale", false)); !ok {
		t.Error("con la capacità struttura parziale (lettura troncata) la struttura c'è")
	}
}

// ---- la regola 1: compatibilità parziale e namespace ----

var selPuntiStrutture = []string{"radice_step.id", "radice_step.nome", "nodo_step.id", "nodo_step.nome"}

// famPuntiStrutture: la base a punti ACME (prefisso 9, gruppo, numero, cifra T), con la forma completa e la forma
// parziale senza la T (A-C07), sui campi dello STEP.
func famPuntiStrutture() grammatica.FamigliaCodice {
	parolaPunto := grammatica.ConfineParolaASCIIOPuntoCifra
	parola := grammatica.ConfineParolaASCII
	pBaseS := []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}
	return grammatica.FamigliaCodice{
		ID: "acme-punti-step", Namespace: "acme-punti-step",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(grammatica.SegmentoBase{Nome: "prefisso", Letterale: "9", Identitario: true},
			grammatica.SegmentoBase{Nome: "gruppo", Pattern: "[0-9]{3}", Separatore: ".", Identitario: true},
			grammatica.SegmentoBase{Nome: "numero", Pattern: "[0-9]{4}", Separatore: ".", Identitario: true},
			grammatica.SegmentoBase{Nome: "T", Pattern: "[0-9]", Separatore: ".", Identitario: true}),
		Forme: []grammatica.FormaCodice{
			{ID: "completa", Selettori: selPuntiStrutture, Stato: grammatica.StatoAttiva, Completa: true, Parti: pBaseS, ConfineDopo: &parola},
			{ID: "parziale", Selettori: selPuntiStrutture, Stato: grammatica.StatoAttiva, Completa: false, SegmentiMancanti: []string{"T"},
				Parti: pBaseS, ConfineDopo: &parolaPunto},
		},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-completa", "radice_step.id", "9.123.4567.3", false, grammatica.LetturaAttesa{Forma: "completa", Base: "9.123.4567.3"}),
			esempio("e-parziale", "radice_step.id", "9.123.4567", false, grammatica.LetturaAttesa{Forma: "parziale", Base: "9.123.4567", Mancanti: []string{"T"}}),
		},
	}
}

// famAltroNamespace: la stessa base 712 più quattro cifre in un altro spazio di codici, riconosciuta nello STEP solo
// con il prefisso X attaccato: un nodo «X7120700» lo legge solo questa famiglia.
func famAltroNamespace() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-altro", Namespace: "acme-altro",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "step", Selettori: selStrutture, Stato: grammatica.StatoAttiva, Completa: true, Parti: []grammatica.Parte{
			{Tipo: grammatica.TipoParteAffisso, Rif: "X", Min: 1, Max: 1}, {Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Affissi: []grammatica.Affisso{{ID: "X", Letterali: []string{"X"}, Posizione: grammatica.PosizionePrefisso, Riconoscimento: selStrutture}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-step", "radice_step.id", "X7120700", false, grammatica.LetturaAttesa{Forma: "step", Base: "7120700", Affissi: []string{"X"}}),
		},
	}
}

// TestRegola1CompatibileParzialeENamespace (6.4.5 regola 1; A-C07; R76 b): una radice letta con la forma parziale,
// compatibile con la base completa del target, è una radice candidata, con la compatibilità «compatibile_parziale»;
// una parziale con un segmento diverso e una completa diversa non lo sono. Un nodo letto da una famiglia di un altro
// namespace, con la stessa base, non è una radice candidata: le basi si confrontano solo nello stesso namespace.
func TestRegola1CompatibileParzialeENamespace(t *testing.T) {
	t.Run("compatibile parziale", func(t *testing.T) {
		m := motore(t, famPuntiStrutture())
		file := func(n int, sha, id string) ancoraggio.StrutturaFile {
			return strutturaS(t, fileS(t, m, uidS(n), "assieme-"+sha[:1]+".stp", "stp", sha, fattiS(t, sha, []string{"#1"},
				[]nodoS{{"#1", id, "ASSIEME"}, {"#2", "9.123.9999.1", "PEZZO"}}, []arcoS{{"#1", "#2", 1, nil}}, testoS(""))))
		}
		parziale, altraParziale, altraCompleta := file(0x5c1, shaS("a"), "9.123.4567"), file(0x5c2, shaS("b"), "9.123.4568"), file(0x5c3, shaS("c"), "9.123.4567.5")
		tg := targetS(t, m, "identificativo:9.123.4567.3", ancoraggio.AutoritaConfermata, "9.123.4567.3")
		if !tg.Base.Completa || tg.Namespace != "acme-punti-step" {
			t.Fatalf("il target deve essere letto completo: %+v", tg)
		}
		strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{
			Strutture: []ancoraggio.StrutturaFile{parziale, altraParziale, altraCompleta}})
		if len(strutture) != 1 || len(diag) != 0 {
			t.Fatalf("attesa la sola struttura della radice parziale compatibile: %+v %+v", strutture, diag)
		}
		x := strutture[0]
		if x.Sha256 != shaS("a") || x.Compatibilita != motorea.CompatibilitaParziale || !x.RadiceDelFile || x.Stato != ancoraggio.StatoStrutturaCandidata ||
			len(x.Fonti) != 1 || x.Fonti[0].Compatibilita != motorea.CompatibilitaParziale {
			t.Errorf("la struttura della radice parziale: %+v", x)
		}
	})
	t.Run("un altro namespace con la stessa base", func(t *testing.T) {
		m := motore(t, famStrutture(), famAltroNamespace())
		sha := shaS("d")
		s := stepS(t, m, uidS(0x5c4), sha, []nodoS{{"#1", "X7120700", "ASSIEME"}, {"#2", "7120710", "PEZZO"}}, []arcoS{{"#1", "#2", 1, nil}}, "")
		var radice ancoraggio.NodoStruttura
		for _, n := range s.Nodi {
			if n.Chiave == "#1" {
				radice = n
			}
		}
		if len(radice.Letture) != 1 || radice.Letture[0].Forma.Namespace != "acme-altro" || radice.Letture[0].Forma.Base.Normalizzata != "7120700" {
			t.Fatalf("la radice deve essere letta solo dall'altro namespace, con la stessa base: %+v", radice.Letture)
		}
		tg := targetS(t, m, "identificativo:7120700", ancoraggio.AutoritaConfermata, "7120700")
		if tg.Namespace != "acme-strutture" || tg.Base.Normalizzata != "7120700" {
			t.Fatalf("il target nel suo namespace: %+v", tg)
		}
		strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
		if len(strutture) != 0 || len(conCodice(diag, ancoraggio.CodiceTargetSenzaStruttura)) != 1 {
			t.Errorf("un nodo di un altro namespace non è una radice candidata: %+v %+v", strutture, diag)
		}
	})
}

// ---- A1c-L1-07: la scena ACME, parte delle strutture ----

// TestL107LeStruttureDellaScena (A1c-L1-07, le parti delle strutture; 6.4.5 regole 1 e 2; R59 A): due assiemi chiesti
// con l'autorità dello scenario e uno non chiesto. Ogni target ha una struttura candidata, quella del suo STEP sotto la
// radice del file, con i figli condivisi e non condivisi; l'assieme non chiesto e i pezzi «solo parti» non sono
// strutture di nessun target; nessuna BOM di lavoro (lo scenario non ha una fonte confermata); i due STEP dei target
// sono documenti candidati della fonte, con la radice (R76 b A).
func TestL107LeStruttureDellaScena(t *testing.T) {
	m := motoreStrutture(t)
	target := targetScenaACME(t, m)
	strutture, diag := proponiS(t, target, ancoraggio.ContestoStrutturale{Strutture: scenaACME(t, m, "")})

	if len(strutture) != 2 || len(diag) != 0 {
		t.Fatalf("attese due strutture e nessuna diagnostica: %d %+v", len(strutture), diag)
	}
	for _, c := range []struct {
		target, sha string
		allegato    uuid.UUID
		figli       []string
	}{
		{"scenario:caso-acme:1", shaAssieme100, idAssieme100, []string{"#2", "#3", "#4", "#5"}},
		{"scenario:caso-acme:2", shaAssieme103, idAssieme103, []string{"#2", "#3", "#4"}},
	} {
		s := struttureDi(strutture, c.target)
		if len(s) != 1 {
			t.Fatalf("%s: %d strutture", c.target, len(s))
		}
		x := s[0]
		attesi := []string{ancoraggio.RifNodo(c.sha, "#1")}
		for _, k := range c.figli {
			attesi = append(attesi, ancoraggio.RifNodo(c.sha, k))
		}
		if x.AllegatoID != c.allegato || x.Sha256 != c.sha || !x.RadiceDelFile || x.Stato != ancoraggio.StatoStrutturaCandidata ||
			x.Compatibilita != motorea.CompatibilitaUguale || !x.GrafoCompleto || !reflect.DeepEqual(rifNodi(x), attesi) {
			t.Errorf("%s: struttura %+v", c.target, x)
		}
		if len(x.Fonti) != 1 || x.Fonti[0].Origine != ancoraggio.CandidatoDaMotoreA || x.Fonti[0].RadiceCompatibile != "#1" ||
			x.Fonti[0].Sha256 != c.sha || *x.Fonti[0].AllegatoID != c.allegato || x.Fonti[0].Estrazione != ancoraggio.EstrazioneRiuscita {
			t.Errorf("%s: fonti %+v", c.target, x.Fonti)
		}
		if got := ancoraggio.StatoStrutturaDelTarget(strutture, c.target); got != ancoraggio.StatoStrutturaCandidata {
			t.Errorf("%s: stato %s, senza fonte confermata tutto è candidato (R59 A)", c.target, got)
		}
	}
	// Il pezzo 7120112 sta in tutti e due gli assiemi: due nodi, uno per file, mai fusi per codice (T-E1-04).
	a, b := struttureDi(strutture, "scenario:caso-acme:1")[0], struttureDi(strutture, "scenario:caso-acme:2")[0]
	n1, ok1 := nodoDi(a, ancoraggio.RifNodo(shaAssieme100, "#4"))
	n2, ok2 := nodoDi(b, ancoraggio.RifNodo(shaAssieme103, "#4"))
	if !ok1 || !ok2 || n1.Rif == n2.Rif {
		t.Errorf("il figlio in comune: %+v %+v", n1, n2)
	}
	// La quantità e le occorrenze del figlio con due occorrenze.
	for _, x := range a.Archi {
		if x.Figlio == ancoraggio.RifNodo(shaAssieme100, "#4") && (x.Quantita == nil || *x.Quantita != 2 || !reflect.DeepEqual(x.Occorrenze, []string{"#23", "#24"})) {
			t.Errorf("arco del figlio con due occorrenze: %+v", x)
		}
	}
	for _, s := range strutture {
		if s.AllegatoID == idAssieme102 || s.AllegatoID == idPezzo110 || s.AllegatoID == idPezzo112 {
			t.Errorf("una struttura per un file che non ha la radice di un target: %+v", s)
		}
	}
}

// ---- A1c-L1-08: non determinabile, parte delle strutture ----

// TestL108StruttureIncompleteOAssenti (A1c-L1-08, le parti delle strutture; 6.4.5 regola 2; E-20): la struttura di un
// target con il grafo incompleto dà ancoraggio.grafo_incompleto con il motivo; un target senza STEP e un target che la
// grammatica non legge danno ancoraggio.target_senza_struttura, e il loro stato è «nessuna»; i file «solo parti» dei
// figli non rendono incompleta la struttura di nessun target.
func TestL108StruttureIncompleteOAssenti(t *testing.T) {
	m := motoreStrutture(t)
	target := append(targetScenaACME(t, m),
		targetS(t, m, "scenario:caso-acme:3", ancoraggio.AutoritaScenario, "P7120104"),
		targetS(t, m, "identificativo:ACME-XYZ", ancoraggio.AutoritaConfermata, "ACME-XYZ"))
	if len(target[3].Base.Segmenti) != 0 {
		t.Fatalf("il codice inventato non deve essere letto: %+v", target[3].Base)
	}
	strutture, diag := proponiS(t, target, ancoraggio.ContestoStrutturale{Strutture: scenaACME(t, m, "lettura troncata: nodi")})

	incompleti := conCodice(diag, ancoraggio.CodiceGrafoIncompleto)
	if len(incompleti) != 1 || !reflect.DeepEqual(incompleti[0].Rif, []string{"scenario:caso-acme:1", "allegato:" + idAssieme100.String(), ancoraggio.RifNodo(shaAssieme100, "#1")}) ||
		!strings.Contains(incompleti[0].Messaggio, "lettura troncata: nodi") {
		t.Errorf("grafo incompleto: %+v", incompleti)
	}
	if s := struttureDi(strutture, "scenario:caso-acme:1"); len(s) != 1 || s[0].GrafoCompleto || s[0].MotivoGrafo != "lettura troncata: nodi" {
		t.Errorf("la struttura del target con il grafo incompleto: %+v", s)
	}
	senza := conCodice(diag, ancoraggio.CodiceTargetSenzaStruttura)
	if len(senza) != 2 || senza[0].Rif[0] != "identificativo:ACME-XYZ" || senza[1].Rif[0] != "scenario:caso-acme:3" || !strings.Contains(senza[0].Messaggio, "non è letto") {
		t.Errorf("target senza struttura: %+v", senza)
	}
	for _, rif := range []string{"scenario:caso-acme:3", "identificativo:ACME-XYZ"} {
		if got := ancoraggio.StatoStrutturaDelTarget(strutture, rif); got != ancoraggio.StatoStrutturaNessuna {
			t.Errorf("%s: stato %s, atteso nessuna", rif, got)
		}
	}
	for _, d := range diag {
		for _, r := range d.Rif {
			if r == "allegato:"+idPezzo110.String() || r == "allegato:"+idPezzo112.String() {
				t.Errorf("un file «solo parti» di un figlio in una diagnostica delle strutture: %+v", d)
			}
		}
	}
}

// ---- A1c-L1-09: il figlio condiviso, parte delle strutture ----

// TestL109FiglioCondivisoNellaStruttura (A1c-L1-09, le parti delle strutture; FIGLIO-CONDIVISO; v3 §2 r.162): un DAG
// a due livelli; il figlio sotto due padri è un nodo solo, con i due padri immediati (diversi dalla radice), e i due
// archi conservano quantità e occorrenze; il nipote si raggiunge una volta.
func TestL109FiglioCondivisoNellaStruttura(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("a")
	s := strutturaS(t, fileS(t, m, uidS(0x530), "ACME-030P7120200 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{"#1"},
		[]nodoS{{"#1", "7120200", "GRUPPO"}, {"#2", "7120210", "LATO DX"}, {"#3", "7120220", "LATO SX"}, {"#4", "7120230", "PERNO"}, {"#6", "7120240", "RONDELLA"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#2", "#4", 2, []string{"#13", "#14"}}, {"#3", "#4", 1, []string{"#15"}}, {"#4", "#6", 3, nil}},
		testoS(""))))
	tg := targetS(t, m, "componente:"+uidS(0x531).String(), ancoraggio.AutoritaConfermata, "7120200")
	strutture, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if len(strutture) != 1 {
		t.Fatalf("attesa una struttura: %+v", strutture)
	}
	x := strutture[0]
	r := func(k string) string { return ancoraggio.RifNodo(sha, k) }
	if !reflect.DeepEqual(rifNodi(x), []string{r("#1"), r("#2"), r("#3"), r("#4"), r("#6")}) {
		t.Errorf("nodi %v: il figlio condiviso è uno solo", rifNodi(x))
	}
	z, _ := nodoDi(x, r("#4"))
	if !reflect.DeepEqual(z.Padri, []string{r("#2"), r("#3")}) {
		t.Errorf("padri immediati del figlio condiviso: %v", z.Padri)
	}
	w, _ := nodoDi(x, r("#6"))
	if !reflect.DeepEqual(w.Padri, []string{r("#4")}) {
		t.Errorf("padri del nipote: %v", w.Padri)
	}
	due, uno, tre := 2, 1, 3
	attesi := []ancoraggio.ArcoProposto{
		{Padre: r("#1"), Figlio: r("#2"), Quantita: &uno}, {Padre: r("#1"), Figlio: r("#3"), Quantita: &uno},
		{Padre: r("#2"), Figlio: r("#4"), Quantita: &due, Occorrenze: []string{"#13", "#14"}},
		{Padre: r("#3"), Figlio: r("#4"), Quantita: &uno, Occorrenze: []string{"#15"}},
		{Padre: r("#4"), Figlio: r("#6"), Quantita: &tre},
	}
	if !reflect.DeepEqual(x.Archi, attesi) {
		t.Errorf("archi, quantità e percorrenze:\n%+v\nattesi:\n%+v", x.Archi, attesi)
	}
}

// ---- A1c-L1-20: figli senza file e nodi senza lettura ----

// TestL120FigliSenzaFileENodiSenzaLettura (A1c-L1-20, la parte di StrutturaProdotto, riscritta senza il nome di
// prima; 3.3.7, P-20; D10): nella struttura di un target ogni figlio si mostra, anche senza file ancorati (SenzaFile,
// senza nessuna diagnostica: l'assenza è ammessa) e anche quando nessuna famiglia lo legge (Leggibile falso); un nodo
// senza lettura non è mai la radice di una struttura.
func TestL120FigliSenzaFileENodiSenzaLettura(t *testing.T) {
	m := motoreStrutture(t)
	strutture, diag := proponiS(t, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: scenaACME(t, m, "")})
	if len(diag) != 0 {
		t.Errorf("figli senza file e nodi senza lettura non danno diagnostiche: %+v", diag)
	}
	x := struttureDi(strutture, "scenario:caso-acme:1")[0]
	saldatura, ok := nodoDi(x, ancoraggio.RifNodo(shaAssieme100, "#5"))
	if !ok || saldatura.Leggibile || !saldatura.SenzaFile || !reflect.DeepEqual(saldatura.Padri, []string{ancoraggio.RifNodo(shaAssieme100, "#1")}) {
		t.Errorf("la saldatura si mostra, senza lettura: %+v", saldatura)
	}
	for _, n := range x.Nodi {
		if n.Rif != saldatura.Rif && !n.Leggibile {
			t.Errorf("un nodo letto dice Leggibile falso: %+v", n)
		}
		if !n.SenzaFile {
			t.Errorf("un nodo con un file ancorato, ma le strutture non ancorano file: %+v", n)
		}
	}
	for _, s := range strutture {
		if s.Radice == saldatura.Rif {
			t.Errorf("un nodo senza lettura è radice di una struttura: %+v", s)
		}
	}
}

// ---- PO-05 e R59: la struttura candidata, la BOM di lavoro sotto la radice scelta ----

// scenaR59: il prodotto 7120300 in tre STEP: il primo con due radici dello stesso codice (due configurazioni), con
// un sottoassieme e un pezzo; il secondo con la sola radice e un figlio; il terzo è lo stesso contenuto del primo in
// un altro allegato. Più uno STEP di un altro prodotto, con un nodo interno di un altro codice.
var (
	idR59a, idR59b, idR59c, idR59d = uidS(0x541), uidS(0x542), uidS(0x543), uidS(0x544)
	shaR59a, shaR59b, shaR59d      = shaS("6"), shaS("7"), shaS("8")
	componenteR59                  = uidS(0x545)
)

func scenaR59(t *testing.T, m *motorea.Motore) []ancoraggio.StrutturaFile {
	t.Helper()
	nodiA := []nodoS{{"#1", "7120300", "CONFIGURAZIONE DX"}, {"#2", "7120310", "SOTTOASSIEME"}, {"#3", "7120311", "PEZZO"},
		{"#60", "7120300", "CONFIGURAZIONE SX"}, {"#61", "7120312", "PEZZO SX"}}
	archiA := []arcoS{{"#1", "#2", 1, nil}, {"#2", "#3", 4, []string{"#71", "#72", "#73", "#74"}}, {"#60", "#61", 1, nil}}
	dueRadici := "2 radici nel file: una distinta ne ha una"
	return []ancoraggio.StrutturaFile{
		strutturaS(t, fileS(t, m, idR59a, "ACME-030P7120300 00 IN_WORK.stp", "stp", shaR59a, fattiS(t, shaR59a, []string{"#1", "#60"}, nodiA, archiA, &dueRadici))),
		stepS(t, m, idR59b, shaR59b, []nodoS{{"#1", "7120300", "VERSIONE B"}, {"#2", "7120320", "PIASTRA"}}, []arcoS{{"#1", "#2", 2, nil}}, ""),
		strutturaS(t, fileS(t, m, idR59c, "copia ACME-030P7120300.stp", "stp", shaR59a, fattiS(t, shaR59a, []string{"#1", "#60"}, nodiA, archiA, &dueRadici))),
		stepS(t, m, idR59d, shaR59d, []nodoS{{"#1", "7120400", "ALTRO"}, {"#2", "GRUPPO_A", "RAGGRUPPAMENTO"}, {"#3", "7120410", "PEZZO"}},
			[]arcoS{{"#1", "#2", 1, nil}, {"#2", "#3", 1, nil}}, ""),
	}
}

func targetR59(t *testing.T, m *motorea.Motore, fonte *ancoraggio.RiferimentoFonte) ancoraggio.ProdottoRichiesto {
	t.Helper()
	tg := targetS(t, m, ancoraggio.RifComponente(componenteR59), ancoraggio.AutoritaConfermata, "7120300")
	c := componenteR59
	tg.ComponenteID, tg.FonteConfermata = &c, fonte
	return tg
}

func fonteR59(sha, radice string, allegato *uuid.UUID) *ancoraggio.RiferimentoFonte {
	da := uidS(0xa1)
	return &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: uidS(0x546), Sha256: sha, AllegatoID: allegato,
		Radice: radice, Ruolo: "radice", Forma: ancoraggio.FormaRiferimentoSmistamento, ConfermatoDa: &da, ConfermatoIl: "2026-10-02T09:00:00Z"}
}

// TestPO05SenzaFonteConfermataSoloStruttureCandidate (PO-05, la parte di B2; R59 A, R85): senza uno STEP strutturale
// confermato ci sono solo strutture candidate, una per STEP e radice, con i loro documenti candidati; nessuna BOM di
// lavoro; un target senza nessuno STEP ha lo stato «nessuna». La fonte del prodotto resta quella di valutazione (B1:
// assente o in attesa di conferma): qui non si tocca.
func TestPO05SenzaFonteConfermataSoloStruttureCandidate(t *testing.T) {
	m := motoreStrutture(t)
	senzaStep := targetS(t, m, "componente:"+uidS(0x547).String(), ancoraggio.AutoritaConfermata, "7120900")
	strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, nil), senzaStep}, ancoraggio.ContestoStrutturale{Strutture: scenaR59(t, m)})
	proprie := struttureDi(strutture, ancoraggio.RifComponente(componenteR59))
	if len(proprie) != 5 {
		t.Fatalf("attese cinque strutture candidate (due radici in due allegati con lo stesso contenuto, più il secondo STEP): %d", len(proprie))
	}
	for _, s := range proprie {
		if s.Stato != ancoraggio.StatoStrutturaCandidata || len(s.Fonti) != 1 {
			t.Errorf("senza fonte confermata ogni struttura è candidata, con il suo documento candidato: %+v", s)
		}
	}
	if ancoraggio.StatoStrutturaDelTarget(strutture, ancoraggio.RifComponente(componenteR59)) != ancoraggio.StatoStrutturaCandidata ||
		ancoraggio.StatoStrutturaDelTarget(strutture, senzaStep.Rif) != ancoraggio.StatoStrutturaNessuna {
		t.Error("stati dei target: candidata e nessuna")
	}
	if len(conCodice(diag, ancoraggio.CodiceTargetSenzaStruttura)) != 1 {
		t.Errorf("il target senza STEP: %+v", diag)
	}
}

// TestR59LaBOMDiLavoroSoloSottoLaRadiceScelta (R59 A, R76 A, R85; A2.3 lo riverificherà): prima del gesto tutto è
// candidato; con la fonte confermata (sha256 e radice), nello stesso calcolo, solo la struttura sotto la radice scelta
// diventa BOM di lavoro proposta, con la stessa gerarchia e le stesse quantità della candidata; la radice diversa
// dello stesso STEP, il secondo STEP dello stesso prodotto e lo stesso contenuto in un altro allegato restano
// candidati; lo STEP della fonte confermata non è più un documento candidato (T-B0-07).
func TestR59LaBOMDiLavoroSoloSottoLaRadiceScelta(t *testing.T) {
	m := motoreStrutture(t)
	ctx := ancoraggio.ContestoStrutturale{Strutture: scenaR59(t, m)}
	rif := ancoraggio.RifComponente(componenteR59)
	prima, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, nil)}, ctx)
	dopo, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, fonteR59(shaR59a, "#1", &idR59a))}, ctx)

	if len(prima) != len(dopo) {
		t.Fatalf("il gesto non aggiunge né toglie strutture: %d contro %d", len(prima), len(dopo))
	}
	nBOM := 0
	for i := range dopo {
		p, d := prima[i], dopo[i]
		sotto := d.AllegatoID == idR59a && d.Radice == ancoraggio.RifNodo(shaR59a, "#1")
		switch {
		case sotto:
			nBOM++
			if d.Stato != ancoraggio.StatoBOMDiLavoroProposta {
				t.Errorf("la struttura sotto la radice scelta: %s", d.Stato)
			}
		case d.Stato != ancoraggio.StatoStrutturaCandidata:
			t.Errorf("una struttura fuori dalla radice scelta non è candidata: %+v", d)
		}
		// La stessa gerarchia, le stesse quantità: cambia solo lo stato e, per lo STEP confermato, le fonti.
		p.Stato, d.Stato, p.Fonti, d.Fonti = "", "", nil, nil
		if canonico(t, p) != canonico(t, d) {
			t.Errorf("la struttura cambia con il gesto oltre allo stato:\n%s\n%s", canonico(t, p), canonico(t, d))
		}
		if dopo[i].Sha256 == shaR59a && len(dopo[i].Fonti) != 0 {
			t.Errorf("lo STEP della fonte confermata resta fra i documenti candidati: %+v", dopo[i].Fonti)
		}
		if dopo[i].Sha256 == shaR59b && len(dopo[i].Fonti) != 1 {
			t.Errorf("il secondo STEP del prodotto resta un documento candidato: %+v", dopo[i].Fonti)
		}
	}
	if nBOM != 1 || ancoraggio.StatoStrutturaDelTarget(dopo, rif) != ancoraggio.StatoBOMDiLavoroProposta {
		t.Errorf("una sola BOM di lavoro: %d", nBOM)
	}
	bom := struttureDi(dopo, rif)
	for _, s := range bom {
		if s.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			uno, quattro := 1, 4
			attesi := []ancoraggio.ArcoProposto{
				{Padre: ancoraggio.RifNodo(shaR59a, "#1"), Figlio: ancoraggio.RifNodo(shaR59a, "#2"), Quantita: &uno},
				{Padre: ancoraggio.RifNodo(shaR59a, "#2"), Figlio: ancoraggio.RifNodo(shaR59a, "#3"), Quantita: &quattro, Occorrenze: []string{"#71", "#72", "#73", "#74"}},
			}
			if !reflect.DeepEqual(s.Archi, attesi) || len(s.Nodi) != 3 {
				t.Errorf("la gerarchia della BOM di lavoro: %+v", s)
			}
		}
	}
	if len(conCodice(diag, ancoraggio.CodiceGrafoIncompleto)) != 4 {
		t.Errorf("le quattro strutture dello STEP con due radici hanno il grafo incompleto: %+v", diag)
	}

	t.Run("senza l'allegato nel riferimento: il primo allegato con quel contenuto", func(t *testing.T) {
		s, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, fonteR59(shaR59a, "#1", nil))}, ctx)
		for _, x := range s {
			if x.Stato == ancoraggio.StatoBOMDiLavoroProposta && x.AllegatoID != idR59a {
				t.Errorf("la BOM di lavoro sta nell'allegato %s", x.AllegatoID)
			}
		}
	})
	t.Run("l'allegato del riferimento vince a parità di contenuto", func(t *testing.T) {
		s, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, fonteR59(shaR59a, "#60", &idR59c))}, ctx)
		var n int
		for _, x := range s {
			if x.Stato == ancoraggio.StatoBOMDiLavoroProposta {
				n++
				if x.AllegatoID != idR59c || x.Radice != ancoraggio.RifNodo(shaR59a, "#60") {
					t.Errorf("la BOM di lavoro: %+v", x)
				}
			}
		}
		if n != 1 {
			t.Errorf("BOM di lavoro: %d", n)
		}
	})
	t.Run("radice non registrata, fonte superata, STEP non letto, radice che non c'è: tutto candidato", func(t *testing.T) {
		superata := fonteR59(shaR59a, "#1", &idR59a)
		superata.Superato = true
		for nome, f := range map[string]*ancoraggio.RiferimentoFonte{
			"radice non registrata (T-B0-08)": fonteR59(shaR59a, "", &idR59a),
			"fonte superata":                  superata,
			"STEP confermato senza struttura": fonteR59(shaS("e"), "#1", nil),
			"radice che nel file non c'è":     fonteR59(shaR59a, "#99", &idR59a),
		} {
			s, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, f)}, ctx)
			if got := ancoraggio.StatoStrutturaDelTarget(s, rif); got != ancoraggio.StatoStrutturaCandidata || len(s) != 5 {
				t.Errorf("%s: stato %s, %d strutture", nome, got, len(s))
			}
		}
	})
	t.Run("la radice scelta senza la base del prodotto: la BOM di lavoro si costruisce lo stesso (R76 A)", func(t *testing.T) {
		s, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{targetR59(t, m, fonteR59(shaR59d, "#2", &idR59d))}, ctx)
		var bom []ancoraggio.StrutturaProdotto
		for _, x := range s {
			if x.Stato == ancoraggio.StatoBOMDiLavoroProposta {
				bom = append(bom, x)
			}
		}
		if len(bom) != 1 || bom[0].AllegatoID != idR59d || bom[0].RadiceDelFile || bom[0].Compatibilita != motorea.CompatibilitaNonDeterminabile ||
			!reflect.DeepEqual(rifNodi(bom[0]), []string{ancoraggio.RifNodo(shaR59d, "#2"), ancoraggio.RifNodo(shaR59d, "#3")}) || len(bom[0].Fonti) != 0 {
			t.Errorf("la BOM di lavoro sotto un raggruppamento: %+v", bom)
		}
		if len(s) != 6 {
			t.Errorf("le cinque candidate restano, più la BOM di lavoro: %d", len(s))
		}
	})
}

// ---- il prodotto come nodo interno ----

// TestProdottoComeNodoInterno (R76 b A; 6.4.5 regola 1): un prodotto che compare solo come nodo interno di uno STEP
// più grande ha una struttura candidata con i nodi del suo sottoalbero; la radice è il nodo interno, senza padri
// nella struttura; il file non è un documento candidato della sua fonte, che resta quella di valutazione.
func TestProdottoComeNodoInterno(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("b")
	s := stepS(t, m, uidS(0x550), sha,
		[]nodoS{{"#1", "7120500", "MACCHINA"}, {"#2", "7120510", "GRUPPO"}, {"#3", "7120511", "PEZZO A"}, {"#4", "7120512", "PEZZO B"}, {"#5", "7120520", "TELAIO"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#2", "#3", 2, nil}, {"#2", "#4", 1, nil}, {"#1", "#5", 1, nil}}, "")
	interno := targetS(t, m, "identificativo:7120510", ancoraggio.AutoritaConfermata, "7120510")
	strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{interno}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if len(strutture) != 1 || len(diag) != 0 {
		t.Fatalf("una struttura, nessuna diagnostica: %+v %+v", strutture, diag)
	}
	x := strutture[0]
	if x.RadiceDelFile || x.Stato != ancoraggio.StatoStrutturaCandidata || len(x.Fonti) != 0 || x.Radice != ancoraggio.RifNodo(sha, "#2") ||
		!reflect.DeepEqual(rifNodi(x), []string{ancoraggio.RifNodo(sha, "#2"), ancoraggio.RifNodo(sha, "#3"), ancoraggio.RifNodo(sha, "#4")}) || len(x.Archi) != 2 {
		t.Errorf("il prodotto come nodo interno: %+v", x)
	}
}

// ---- il contesto ----

// TestIlContestoConLaSuaOrigine (6.4.5 regola 2; R29 e, R61 A, R80; emendamento E1 §4.2): accanto all'albero dei fatti
// la struttura porta gli archi del contesto, ognuno con la sua origine: le relazioni confermate raggiungibili dal
// componente del target e gli archi aperti di relazione_proposta del file, mai fusi con i nodi proposti. La riga
// aperta della radice si mostra a parte e non conta fra le righe da decidere; le righe di un altro allegato non
// contano.
func TestIlContestoConLaSuaOrigine(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("c")
	s := stepS(t, m, uidS(0x560), sha, []nodoS{{"#1", "7120600", "PRODOTTO"}, {"#2", "7120610", "STAFFA"}, {"#3", "7120620", "PERNO"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#2", "#3", 2, nil}}, "")
	c1, c2, c3, c8, c9 := uidS(0x561), uidS(0x562), uidS(0x563), uidS(0x568), uidS(0x569)
	tg := targetS(t, m, ancoraggio.RifComponente(c1), ancoraggio.AutoritaConfermata, "7120600")
	tg.ComponenteID = &c1
	due, uno, cinque := 2, 1, 5
	riga := func(id uuid.UUID, allegato uuid.UUID, sha, chiave string) ancoraggio.RigaPropostaLegacy {
		return ancoraggio.RigaPropostaLegacy{ID: id, AllegatoID: allegato, Sha256: sha, Chiave: chiave, Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
	}
	deciso := func(id uuid.UUID, codice string) ancoraggio.ComponenteDeciso {
		return ancoraggio.ComponenteDeciso{ComponenteID: id, Codice: codice, Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato}
	}
	ctx := ancoraggio.ContestoStrutturale{
		Strutture:  []ancoraggio.StrutturaFile{s},
		Confermato: []ancoraggio.ComponenteDeciso{deciso(c1, "7120600"), deciso(c2, "7120610"), deciso(c3, "7120620"), deciso(c8, "7120680"), deciso(c9, "7120690")},
		ArchiConfermati: []ancoraggio.ArcoPercorso{
			{Padre: ancoraggio.RifComponente(c2), Figlio: ancoraggio.RifComponente(c3), Quantita: &due, Origine: ancoraggio.OrigineArcoConfermato},
			{Padre: ancoraggio.RifComponente(c1), Figlio: ancoraggio.RifComponente(c2), Quantita: &uno, Origine: ancoraggio.OrigineArcoConfermato},
			{Padre: ancoraggio.RifComponente(c9), Figlio: ancoraggio.RifComponente(c8), Quantita: &cinque, Origine: ancoraggio.OrigineArcoConfermato},
		},
		Proposto: []ancoraggio.RigaPropostaLegacy{
			riga(uidS(0x571), s.AllegatoID, sha, "#1"), riga(uidS(0x572), s.AllegatoID, sha, "#3"),
			riga(uidS(0x573), uidS(0x5ff), shaS("d"), "riga 1"),
		},
		ArchiProposti: []ancoraggio.ArcoPercorso{
			{Padre: ancoraggio.RifNodo(sha, "#2"), Figlio: ancoraggio.RifNodo(sha, "#3"), Quantita: &due, Origine: ancoraggio.OrigineArcoProposto},
			{Padre: ancoraggio.RifNodo(shaS("d"), "#1"), Figlio: ancoraggio.RifNodo(shaS("d"), "#2"), Quantita: &uno, Origine: ancoraggio.OrigineArcoProposto},
		},
	}
	strutture, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if len(strutture) != 1 {
		t.Fatalf("una struttura: %+v", strutture)
	}
	x := strutture[0]
	attesi := []ancoraggio.ArcoPercorso{
		{Padre: ancoraggio.RifComponente(c1), Figlio: ancoraggio.RifComponente(c2), Quantita: &uno, Origine: ancoraggio.OrigineArcoConfermato},
		{Padre: ancoraggio.RifComponente(c2), Figlio: ancoraggio.RifComponente(c3), Quantita: &due, Origine: ancoraggio.OrigineArcoConfermato},
		{Padre: ancoraggio.RifNodo(sha, "#2"), Figlio: ancoraggio.RifNodo(sha, "#3"), Quantita: &due, Origine: ancoraggio.OrigineArcoProposto},
	}
	if !reflect.DeepEqual(x.ArchiContesto, attesi) {
		t.Errorf("archi del contesto:\n%+v\nattesi:\n%+v", x.ArchiContesto, attesi)
	}
	if len(x.Nodi) != 3 || len(x.Archi) != 2 {
		t.Errorf("il contesto non aggiunge nodi né archi all'albero dei fatti (R68 A): %+v", x)
	}
	if x.RigaRadice != ancoraggio.RifRigaProposta(uidS(0x571)) || !reflect.DeepEqual(x.RigheDaDecidere, []string{ancoraggio.RifRigaProposta(uidS(0x572))}) {
		t.Errorf("la riga della radice a parte, le altre da decidere (R80): %q %v", x.RigaRadice, x.RigheDaDecidere)
	}
	if x.Stato != ancoraggio.StatoStrutturaCandidata {
		t.Errorf("le relazioni confermate non promuovono la struttura (R61 A): %s", x.Stato)
	}

	t.Run("un target di scenario non ha relazioni confermate", func(t *testing.T) {
		sc := targetS(t, m, "scenario:caso-acme:9", ancoraggio.AutoritaScenario, "P7120600")
		s, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{sc}, ctx)
		for _, a := range s[0].ArchiContesto {
			if a.Origine == ancoraggio.OrigineArcoConfermato {
				t.Errorf("relazione confermata su un target senza componente: %+v", a)
			}
		}
	})
}

// ---- il determinismo ----

// TestStruttureDeterministiche (6.4.5 regola 8, la parte delle strutture; par.3.4.2): gli stessi ingressi in un altro
// ordine (target, strutture, nodi, archi, componenti, righe, letture, e i documenti degli STEP) danno gli stessi byte
// canonici, per StrutturaDa e per ProponiStrutture.
func TestStruttureDeterministiche(t *testing.T) {
	m := motoreStrutture(t)
	sha := shaS("a")
	f := fileS(t, m, uidS(0x580), "ACME-030P7120200 00 IN_WORK.stp", "stp", sha, fattiS(t, sha, []string{"#1"},
		[]nodoS{{"#1", "7120200", "GRUPPO"}, {"#2", "7120210", "LATO DX"}, {"#3", "7120220", "LATO SX"}, {"#4", "7120230", "PERNO"}},
		[]arcoS{{"#1", "#2", 1, nil}, {"#1", "#3", 1, nil}, {"#2", "#4", 2, nil}, {"#3", "#4", 1, nil}}, testoS("")))
	permutato := f
	permutato.Documento.Entita = rovescia(f.Documento.Entita)
	permutato.Documento.Unita = rovescia(f.Documento.Unita)
	permutato.Documento.Legami = rovescia(f.Documento.Legami)
	permutato.Interpretazione.Letture = rovescia(f.Interpretazione.Letture)
	a, b := strutturaS(t, f), strutturaS(t, permutato)
	if canonico(t, a) != canonico(t, b) {
		t.Errorf("StrutturaDa dipende dall'ordine del documento:\n%s\n%s", canonico(t, a), canonico(t, b))
	}

	ctx := ancoraggio.ContestoStrutturale{Strutture: append(scenaACME(t, m, ""), scenaR59(t, m)...)}
	c1, c2 := uidS(0x581), uidS(0x582)
	uno := 1
	ctx.Confermato = []ancoraggio.ComponenteDeciso{
		{ComponenteID: c1, Codice: "7120300", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato},
		{ComponenteID: c2, Codice: "7120310", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato},
	}
	ctx.ArchiConfermati = []ancoraggio.ArcoPercorso{{Padre: ancoraggio.RifComponente(c1), Figlio: ancoraggio.RifComponente(c2), Quantita: &uno, Origine: ancoraggio.OrigineArcoConfermato}}
	ctx.Proposto = []ancoraggio.RigaPropostaLegacy{
		{ID: uidS(0x583), AllegatoID: idR59a, Sha256: shaR59a, Chiave: "#1", Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto},
		{ID: uidS(0x584), AllegatoID: idR59a, Sha256: shaR59a, Chiave: "#2", Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto},
	}
	ctx.ArchiProposti = []ancoraggio.ArcoPercorso{
		{Padre: ancoraggio.RifNodo(shaR59a, "#1"), Figlio: ancoraggio.RifNodo(shaR59a, "#2"), Quantita: &uno, Origine: ancoraggio.OrigineArcoProposto},
		{Padre: ancoraggio.RifNodo(shaR59a, "#2"), Figlio: ancoraggio.RifNodo(shaR59a, "#3"), Origine: ancoraggio.OrigineArcoProposto},
	}
	tr := targetR59(t, m, fonteR59(shaR59a, "#1", &idR59a))
	tr.Rif, tr.ComponenteID = ancoraggio.RifComponente(c1), &c1
	target := append(targetScenaACME(t, m), tr, targetS(t, m, "scenario:caso-acme:3", ancoraggio.AutoritaScenario, "P7120104"))

	rov := ancoraggio.ContestoStrutturale{
		Strutture: rovescia(ctx.Strutture), Confermato: rovescia(ctx.Confermato), ArchiConfermati: rovescia(ctx.ArchiConfermati),
		Proposto: rovescia(ctx.Proposto), ArchiProposti: rovescia(ctx.ArchiProposti),
	}
	for i := range rov.Strutture {
		rov.Strutture[i].Nodi = rovescia(rov.Strutture[i].Nodi)
		rov.Strutture[i].Archi = rovescia(rov.Strutture[i].Archi)
		rov.Strutture[i].Radici = rovescia(rov.Strutture[i].Radici)
	}
	s1, d1 := proponiS(t, target, ctx)
	s2, d2 := proponiS(t, rovescia(target), rov)
	if canonico(t, s1) != canonico(t, s2) || canonico(t, d1) != canonico(t, d2) {
		t.Errorf("ProponiStrutture dipende dall'ordine degli ingressi:\n%s\n%s", canonico(t, s1), canonico(t, s2))
	}
	s3, d3 := proponiS(t, target, ctx)
	if canonico(t, s1) != canonico(t, s3) || canonico(t, d1) != canonico(t, d3) {
		t.Error("due chiamate con gli stessi ingressi danno esiti diversi")
	}
	if len(s1) == 0 || len(d1) == 0 {
		t.Fatalf("la prova del determinismo ha bisogno di strutture e di diagnostiche: %d %d", len(s1), len(d1))
	}
}

// rovescia: una copia dell'elenco, al contrario.
func rovescia[T any](s []T) []T {
	out := make([]T, len(s))
	for i, x := range s {
		out[len(s)-1-i] = x
	}
	return out
}

// ---- gli errori di contratto ----

// TestProponiStruttureErroriDiContratto: un ingresso che viola il contratto è un errore di contratto, con il codice
// della foglia, mai un esito: un target ripetuto, senza Rif o con l'autorità proposta (R60 A); una fonte confermata
// che non è uno STEP (R68 A, T-E1-09); due strutture dello stesso allegato; un nodo con un Rif che non è sha256 più
// chiave (T-E1-04); una struttura senza allegato o sha256, un nodo senza chiave, una riga senza allegato, sha256 o
// chiave (fuori dal suo file la chiave non significa niente: T-E1-04, emendamento E1 §4.2); un arco fuori dai nodi o
// con l'origine sbagliata; un componente non confermato e una riga non proposta (R29 e); una relazione confermata
// ripetuta; un arco proposto fra riferimenti che non sono nodi; una riga di un altro contenuto della struttura del
// suo allegato. Dove serve, la prova guarda anche il percorso, perché il caso cada sul suo controllo.
func TestProponiStruttureErroriDiContratto(t *testing.T) {
	m := motoreStrutture(t)
	s := scenaACME(t, m, "")[1]
	tg := targetScenaACME(t, m)[1]
	ok := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}
	if _, _, err := ancoraggio.ProponiStrutture([]ancoraggio.ProdottoRichiesto{tg}, ok); err != nil {
		t.Fatalf("l'ingresso di base non è valido: %v", err)
	}
	uno := 1
	c := uidS(0x590)
	conTarget := func(f func(*ancoraggio.ProdottoRichiesto)) []ancoraggio.ProdottoRichiesto {
		x := tg
		f(&x)
		return []ancoraggio.ProdottoRichiesto{x}
	}
	conStruttura := func(f func(*ancoraggio.StrutturaFile)) ancoraggio.ContestoStrutturale {
		x := s
		x.Nodi = append([]ancoraggio.NodoStruttura(nil), s.Nodi...)
		x.Archi = append([]ancoraggio.ArcoPercorso(nil), s.Archi...)
		x.Radici = append([]string(nil), s.Radici...)
		f(&x)
		return ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{x}}
	}
	riga := ancoraggio.RigaPropostaLegacy{ID: uidS(0x591), AllegatoID: s.AllegatoID, Sha256: s.Sha256, Chiave: "#1", Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
	confermato := ancoraggio.ArcoPercorso{Padre: ancoraggio.RifComponente(c), Figlio: ancoraggio.RifComponente(uidS(0x592)), Quantita: &uno, Origine: ancoraggio.OrigineArcoConfermato}
	senza := func(f func(*ancoraggio.RigaPropostaLegacy)) ancoraggio.RigaPropostaLegacy {
		r := riga
		f(&r)
		return r
	}
	// Dove deve stare la diagnostica: il caso cade sul suo controllo, non su un altro che passa di lì (per esempio il
	// Rif del nodo, non gli archi che lo nominano; la chiave che manca, non il Rif che non torna).
	percorsi := map[string]string{
		"nodo con il Rif del codice": "contesto.strutture[0].nodi[1].rif",
		"struttura senza sha256":     "contesto.strutture[0].sha256",
		"struttura senza allegato":   "contesto.strutture[0].allegato_id",
		"nodo senza chiave":          "contesto.strutture[0].nodi[3].chiave",
		"riga senza allegato":        "contesto.proposto[0].allegato_id",
		"riga senza sha256":          "contesto.proposto[0].sha256",
		"riga senza chiave":          "contesto.proposto[0].chiave",
	}
	for _, k := range []struct {
		nome   string
		target []ancoraggio.ProdottoRichiesto
		ctx    ancoraggio.ContestoStrutturale
		codice string
	}{
		{"target ripetuto", []ancoraggio.ProdottoRichiesto{tg, tg}, ok, evidenze.CodiceDocumentoIDRipetuto},
		{"target senza Rif", conTarget(func(x *ancoraggio.ProdottoRichiesto) { x.Rif = "" }), ok, evidenze.CodiceDocumentoRiferimentoPendente},
		{"target con l'autorità proposta", conTarget(func(x *ancoraggio.ProdottoRichiesto) { x.Autorita = ancoraggio.AutoritaProposta }), ok, evidenze.CodiceDocumentoEnumIgnoto},
		{"fonte confermata che non è uno STEP", conTarget(func(x *ancoraggio.ProdottoRichiesto) {
			x.FonteConfermata = &ancoraggio.RiferimentoFonte{Tipo: "pdf", Sha256: s.Sha256, Radice: "#1"}
		}), ok, evidenze.CodiceDocumentoEnumIgnoto},
		{"due strutture dello stesso allegato", nil, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s, s}}, evidenze.CodiceDocumentoIDRipetuto},
		{"nodo con il Rif del codice", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Nodi[1].Rif, x.Archi = "nodo:7120122", nil }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"struttura senza sha256", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Sha256 = "" }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"struttura senza allegato", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.AllegatoID = uuid.Nil }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"nodo senza chiave", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Nodi[3].Chiave = "" }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"riga senza allegato", nil, ancoraggio.ContestoStrutturale{Proposto: []ancoraggio.RigaPropostaLegacy{senza(func(r *ancoraggio.RigaPropostaLegacy) { r.AllegatoID = uuid.Nil })}},
			evidenze.CodiceDocumentoRiferimentoPendente},
		{"riga senza sha256", nil, ancoraggio.ContestoStrutturale{Proposto: []ancoraggio.RigaPropostaLegacy{senza(func(r *ancoraggio.RigaPropostaLegacy) { r.Sha256 = "" })}},
			evidenze.CodiceDocumentoRiferimentoPendente},
		{"riga senza chiave", nil, ancoraggio.ContestoStrutturale{Proposto: []ancoraggio.RigaPropostaLegacy{senza(func(r *ancoraggio.RigaPropostaLegacy) { r.Chiave = "" })}},
			evidenze.CodiceDocumentoRiferimentoPendente},
		{"nodo ripetuto", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Nodi = append(x.Nodi, x.Nodi[0]) }), evidenze.CodiceDocumentoIDRipetuto},
		{"radice fuori dai nodi", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Radici = []string{ancoraggio.RifNodo(s.Sha256, "#99")} }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"arco fuori dai nodi", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Archi[0].Figlio = ancoraggio.RifNodo(s.Sha256, "#99") }), evidenze.CodiceDocumentoRiferimentoPendente},
		{"arco ripetuto", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Archi = append(x.Archi, x.Archi[0]) }), evidenze.CodiceDocumentoIDRipetuto},
		{"arco della struttura con un'altra origine", nil, conStruttura(func(x *ancoraggio.StrutturaFile) { x.Archi[0].Origine = ancoraggio.OrigineArcoConfermato }), evidenze.CodiceDocumentoEnumIgnoto},
		{"componente non confermato", nil, ancoraggio.ContestoStrutturale{Confermato: []ancoraggio.ComponenteDeciso{
			{ComponenteID: c, Codice: "7120103", Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}}}, evidenze.CodiceDocumentoEnumIgnoto},
		{"componente ripetuto", nil, ancoraggio.ContestoStrutturale{Confermato: []ancoraggio.ComponenteDeciso{
			{ComponenteID: c, Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato},
			{ComponenteID: c, Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato}}}, evidenze.CodiceDocumentoIDRipetuto},
		{"relazione confermata ripetuta", nil, ancoraggio.ContestoStrutturale{ArchiConfermati: []ancoraggio.ArcoPercorso{confermato, confermato}}, evidenze.CodiceDocumentoIDRipetuto},
		{"relazione confermata fra nodi", nil, ancoraggio.ContestoStrutturale{ArchiConfermati: []ancoraggio.ArcoPercorso{
			{Padre: ancoraggio.RifNodo(s.Sha256, "#1"), Figlio: ancoraggio.RifNodo(s.Sha256, "#2"), Origine: ancoraggio.OrigineArcoConfermato}}}, evidenze.CodiceDocumentoRiferimentoPendente},
		{"relazione confermata con l'origine proposta", nil, ancoraggio.ContestoStrutturale{ArchiConfermati: []ancoraggio.ArcoPercorso{
			{Padre: confermato.Padre, Figlio: confermato.Figlio, Origine: ancoraggio.OrigineArcoProposto}}}, evidenze.CodiceDocumentoEnumIgnoto},
		{"riga confermata fra le aperte", nil, ancoraggio.ContestoStrutturale{Proposto: []ancoraggio.RigaPropostaLegacy{
			{ID: riga.ID, AllegatoID: riga.AllegatoID, Sha256: riga.Sha256, Chiave: "#1", Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato}}}, evidenze.CodiceDocumentoEnumIgnoto},
		{"riga ripetuta", nil, ancoraggio.ContestoStrutturale{Proposto: []ancoraggio.RigaPropostaLegacy{riga, riga}}, evidenze.CodiceDocumentoIDRipetuto},
		{"riga di un altro contenuto", nil, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, Proposto: []ancoraggio.RigaPropostaLegacy{
			{ID: riga.ID, AllegatoID: riga.AllegatoID, Sha256: shaS("e"), Chiave: "#1", Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}}}, evidenze.CodiceDocumentoRiferimentoPendente},
		{"arco proposto fra componenti", nil, ancoraggio.ContestoStrutturale{ArchiProposti: []ancoraggio.ArcoPercorso{
			{Padre: confermato.Padre, Figlio: confermato.Figlio, Origine: ancoraggio.OrigineArcoProposto}}}, evidenze.CodiceDocumentoRiferimentoPendente},
	} {
		t.Run(k.nome, func(t *testing.T) {
			s, d, err := ancoraggio.ProponiStrutture(k.target, k.ctx)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) || len(conCodice(ec.Diagnostiche, k.codice)) == 0 {
				t.Fatalf("errore %v, atteso un errore di contratto con %s", err, k.codice)
			}
			if want, ok := percorsi[k.nome]; ok {
				trovato := false
				for _, x := range conCodice(ec.Diagnostiche, k.codice) {
					trovato = trovato || x.Percorso == want
				}
				if !trovato {
					t.Errorf("nessuna diagnostica %s sul percorso %s: %+v", k.codice, want, ec.Diagnostiche)
				}
			}
			for _, x := range ec.Diagnostiche {
				if x.Gravita != evidenze.GravitaErrore || x.Natura != evidenze.NaturaContratto {
					t.Errorf("diagnostica di contratto: %+v", x)
				}
			}
			if s != nil || d != nil {
				t.Errorf("con l'errore un esito: %+v %+v", s, d)
			}
		})
	}
}

// ---- i valori del contratto ----

// TestIValoriDelleStrutture (contratto §2.2, §2.5; 6.4.5; T-E1-04): i valori delle costanti, la forma dei
// riferimenti e i campi dei tipi delle strutture, con i tag JSON snake_case. Se uno cambia, questa prova si riscrive
// con «Riscritta per …».
//
// Riscritta per B4 (commit P6b, fase 1): i campi che il contratto (§2.2) lasciava al commit di B4 (NodoProposto.Codice,
// Decisione, RigaLegacy, DecisoDaPersona; ArcoProposto.Decisione; ContestoStrutturale.CodiciProposti) e gli ingressi
// della catena e delle decisioni (le righe decise, i codici dei messaggi, le decisioni sull'identità, il codice
// manuale, la revisione e la lettura del componente, i grezzi, le formazioni e il nome del file), più
// NodoProposto.RigaDecisa; ProdottoRichiesto.Marcatore (T-B4-06 rivisto). I valori e i campi dei tipi nuovi di B4 li fissa
// catena_test.go.
//
// Riscritta per B4 (commit P6b, fase 2): NodoProposto.AbbinamentoPerBase (emendamento E1 §4.2, punto 2; lettura
// dell'orchestratore T-B4-12). I valori e i campi dei tipi degli ancoraggi li fissa ancoraggi_test.go.
func TestIValoriDelleStrutture(t *testing.T) {
	id := uidS(0x5a0)
	for _, c := range []struct{ got, want string }{
		{string(ancoraggio.StatoStrutturaNessuna), "nessuna"},
		{string(ancoraggio.StatoStrutturaCandidata), "struttura_candidata"},
		{string(ancoraggio.StatoBOMDiLavoroProposta), "bom_di_lavoro_proposta"},
		{ancoraggio.OrigineArcoFatti, "fatti"},
		{ancoraggio.OrigineArcoConfermato, "confermato"},
		{ancoraggio.OrigineArcoProposto, "proposto"},
		{ancoraggio.CodiceTargetSenzaStruttura, "ancoraggio.target_senza_struttura"},
		{ancoraggio.CodiceGrafoIncompleto, "ancoraggio.grafo_incompleto"},
		{ancoraggio.RifNodo(shaS("a"), "#12"), "nodo:" + shaS("a") + ":#12"},
		{ancoraggio.RifComponente(id), "componente:" + id.String()},
		{ancoraggio.RifRigaProposta(id), "componente_proposta:" + id.String()},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{ancoraggio.StrutturaFile{}, "AllegatoID:allegato_id BundleID:bundle_id Sha256:sha256 Radici:radici Nodi:nodi Archi:archi GrafoCompleto:grafo_completo MotivoGrafo:motivo_grafo " +
			"NomeFile:nome_file"},
		{ancoraggio.NodoStruttura{}, "Rif:rif EntitaID:entita_id Chiave:chiave Letture:letture Grezzi:grezzi Formazioni:formazioni"},
		{ancoraggio.ArcoPercorso{}, "Padre:padre Figlio:figlio Quantita:quantita Occorrenze:occorrenze Origine:origine"},
		{ancoraggio.ProdottoRichiesto{}, "Rif:rif Autorita:autorita ClienteID:cliente_id Namespace:namespace CodiceRichiesto:codice_richiesto Base:base Marcatore:marcatore Revisione:revisione " +
			"Qualificatori:qualificatori Origine:origine VersioneDecisione:versione_decisione ComponenteID:componente_id FonteConfermata:fonte_confermata"},
		// riscritta per la fase 3 di B4: LettureDecise e AssociazioniDecise (T-B4-32, T-B4-33)
		{ancoraggio.ContestoStrutturale{}, "Strutture:strutture Confermato:confermato ArchiConfermati:archi_confermati Proposto:proposto ArchiProposti:archi_proposti " +
			"Decise:decise CodiciProposti:codici_proposti CodiciMessaggi:codici_messaggi DecisioniIdentita:decisioni_identita LettureDecise:letture_decise " +
			"AssociazioniDecise:associazioni_decise"},
		{ancoraggio.ComponenteDeciso{}, "ComponenteID:componente_id Codice:codice Autorita:autorita Origine:origine Rev:rev Lettura:lettura"},
		{ancoraggio.RigaPropostaLegacy{}, "ID:id AllegatoID:allegato_id Sha256:sha256 Chiave:chiave Autorita:autorita Origine:origine " +
			"CodiceManuale:codice_manuale RevManuale:rev_manuale LetturaManuale:lettura_manuale"},
		{ancoraggio.NodoProposto{}, "Rif:rif AllegatoID:allegato_id EntitaID:entita_id Codice:codice Padri:padri Leggibile:leggibile SenzaFile:senza_file " +
			"Decisione:decisione RigaLegacy:riga_legacy RigaDecisa:riga_decisa DecisoDaPersona:deciso_da_persona AbbinamentoPerBase:abbinamento_per_base"},
		{ancoraggio.ArcoProposto{}, "Padre:padre Figlio:figlio Quantita:quantita Occorrenze:occorrenze Decisione:decisione"},
		{ancoraggio.StrutturaProdotto{}, "Target:target AllegatoID:allegato_id Sha256:sha256 Radice:radice RadiceDelFile:radice_del_file Compatibilita:compatibilita " +
			"Stato:stato Nodi:nodi Archi:archi ArchiContesto:archi_contesto Fonti:fonti GrafoCompleto:grafo_completo MotivoGrafo:motivo_grafo " +
			"RigaRadice:riga_radice RigheDaDecidere:righe_da_decidere"},
	} {
		tp := reflect.TypeOf(c.tipo)
		var campi []string
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			campi = append(campi, f.Name+":"+tag)
		}
		if got := strings.Join(campi, " "); got != c.campi {
			t.Errorf("%s: campi %s, il contratto dice %s", tp.Name(), got, c.campi)
		}
	}
}
