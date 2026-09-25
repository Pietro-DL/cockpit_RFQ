package fascicolo

// L1 — B8.5: la classificazione dei nodi (A1.2, D16), il confronto con la working (A1.1, A4.4), i cicli
// e le rimozioni come regole pure. Le prove L4 degli stessi casi, con le tabelle, stanno in
// proposte_db_test.go.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// motoreConFamiglia: una famiglia di otto cifre che comincia per 777, con la revisione dopo il trattino
// basso quando c'e'. E' una famiglia di prova, non quella di un cliente vero.
func motoreConFamiglia(t *testing.T) *classificazione.Motore {
	t.Helper()
	m := classificazione.Compila("Cliente di prova", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>777\d{5})(?:_(?P<rev>[A-Z]))?`, Descrizione: "disegni 777", RevNelCodice: true, Esempio: "77722757_B",
	}}})
	if !m.HaFamiglie() {
		t.Fatal("la famiglia di prova non e' entrata nel motore")
	}
	return m
}

func nodo(chiave, id, nome, descr, rev string) worker.NodoSTEP {
	return worker.NodoSTEP{Chiave: chiave, IDGrezzo: id, NomeGrezzo: nome, DescrizioneGrezza: descr, RevGrezza: rev,
		Evidenza: map[string]any{"entita": "PRODUCT", "riga": chiave}}
}

func TestIlCodiceDelNodoLoDaLaFamigliaDelClienteNonIlWorker(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#12", "", "77722757_B", "SUPPORTO COFANO", "")}}
	n := ClassificaNodi(motoreConFamiglia(t), s)[0]
	if n.Codice != "77722757" || n.Rev != "B" || n.Origine != "famiglia" || n.Famiglia != "disegni 777" {
		t.Fatalf("classificato come %+v", n)
	}
	if n.Confidenza != classificazione.PuntiFamiglia || n.Evidenza["regola"] != "disegni 777" || n.Evidenza["testo"] != "77722757_B" {
		t.Errorf("confidenza o evidenza: %+v", n)
	}
	// lo stesso nodo in una RFQ di un cliente senza famiglie: la lettura cambia, i fatti no
	altro := ClassificaNodi(classificazione.Compila("Altro", regole.Regole{}), s)[0]
	if altro.Origine == "famiglia" || altro.Famiglia != "" {
		t.Errorf("senza famiglie il codice non puo' essere di famiglia: %+v", altro)
	}
}

func TestSenzaFamigliaRestaIlGenerico(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#12", "1234567A", "1234567A", "", "")}}
	n := ClassificaNodi(classificazione.Compila("Senza regole", regole.Regole{}), s)[0]
	if n.Codice == "" || n.Origine != "generico" || n.Confidenza != classificazione.PuntiGenerico {
		t.Fatalf("atteso il generico, trovato %+v", n)
	}
	if nilMotore := ClassificaNodi(nil, s)[0]; nilMotore.Codice != n.Codice {
		t.Errorf("un cliente sconosciuto (motore nil) deve leggere come un cliente senza regole: %+v", nilMotore)
	}
}

func TestUnNomeCheNonEUnCodiceDaUnaPropostaSenzaCodice(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#7", "Part1", "Part1", "", "")}}
	n := ClassificaNodi(motoreConFamiglia(t), s)[0]
	if n.Codice != "" || n.Origine != "" || n.Confidenza != 0 {
		t.Fatalf("«Part1» non e' un codice: %+v", n)
	}
	if n.NomeGrezzo != "Part1" {
		t.Errorf("senza codice il nome grezzo e' l'unica cosa da mostrare: %+v", n)
	}
}

// A1.7: SolidWorks mette il codice in id e il file in name, altri il contrario. Se danno due codici
// diversi vince l'id, e l'altro resta visibile.
func TestSeIdENomeDannoDueCodiciDiversiVinceLId(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", "77720517", "77722757_B", "", "")}}
	n := ClassificaNodi(motoreConFamiglia(t), s)[0]
	if n.Codice != "77720517" || n.Dove != DoveID {
		t.Fatalf("doveva vincere l'id: %+v", n)
	}
	if n.Alternativo != "77722757" || n.Evidenza["alternativo"] != "77722757" {
		t.Errorf("l'altro codice deve restare nell'evidenza: %+v", n)
	}
	// e un codice di famiglia nel nome vale piu' di un generico nell'id
	s = worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", "XX-77", "77722757", "", "")}}
	if n := ClassificaNodi(motoreConFamiglia(t), s)[0]; n.Codice != "77722757" || n.Origine != "famiglia" {
		t.Errorf("la famiglia deve vincere sul generico: %+v", n)
	}
}

func TestLaRevDelFileVinceSuQuellaDellaFamiglia(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{
		nodo("#1", "", "77722757_B", "", "C"),
		nodo("#2", "", "77722758_B", "", ""),
	}}
	n := ClassificaNodi(motoreConFamiglia(t), s)
	if n[0].Rev != "C" || n[1].Rev != "B" {
		t.Errorf("rev = %q e %q, attese C (dal file) e B (dalla famiglia)", n[0].Rev, n[1].Rev)
	}
}

func TestUnCodiceFuoriMisuraNonEntraNelCampo(t *testing.T) {
	lungo := strings.Repeat("7", classificazione.MaxCodice+5)
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", lungo, lungo, "", strings.Repeat("R", 20))}}
	n := ClassificaNodi(classificazione.Compila("x", regole.Regole{}), s)[0]
	if n.Codice != "" || n.Rev != "" {
		t.Fatalf("fuori misura nel campo: %+v", n)
	}
	if n.Evidenza["rev_scartata"] == nil {
		t.Errorf("la revisione scartata deve restare nell'evidenza: %+v", n.Evidenza)
	}
}

// ---------------------------------------------------------------- il confronto con la working

// working costruisce una BOM di prova: componenti per codice e archi (padre, figlio, qta).
type bomProva struct {
	comp map[string]db.Componente
	rel  []db.ComponenteRelazione
}

func nuovaBom(codici ...string) *bomProva {
	b := &bomProva{comp: map[string]db.Componente{}}
	for _, c := range codici {
		b.comp[c] = db.Componente{ComponenteID: uuid.New(), Codice: c, Tipo: db.TipoComponenteSciolto}
	}
	return b
}

func (b *bomProva) arco(padre, figlio string, qta int32) *bomProva {
	b.rel = append(b.rel, db.ComponenteRelazione{PadreID: b.comp[padre].ComponenteID, FiglioID: b.comp[figlio].ComponenteID, Qta: qta})
	return b
}

func (b *bomProva) working() Working {
	var comp []db.Componente
	for _, c := range b.comp {
		comp = append(comp, c)
	}
	return NuovaWorking(comp, b.rel)
}

// file costruisce uno STEP gia' classificato: ogni nodo si chiama come il suo codice.
func file(radici []string, archi ...string) ([]NodoClassificato, worker.StrutturaSTEP) {
	s := worker.StrutturaSTEP{Versione: 3, Radici: radici}
	visti := map[string]bool{}
	var nodi []NodoClassificato
	aggiungi := func(k string) {
		if visti[k] {
			return
		}
		visti[k] = true
		s.Nodi = append(s.Nodi, worker.NodoSTEP{Chiave: k, NomeGrezzo: k})
		nodi = append(nodi, NodoClassificato{Chiave: k, NomeGrezzo: k, Codice: k, Origine: "generico", Evidenza: map[string]any{}})
	}
	for _, r := range radici {
		aggiungi(r)
	}
	for _, a := range archi { // "A>B*2"
		pf, qta := a, 1
		if i := strings.Index(a, "*"); i > 0 {
			pf, qta = a[:i], int(a[i+1]-'0')
		}
		p, f, _ := strings.Cut(pf, ">")
		aggiungi(p)
		aggiungi(f)
		s.Relazioni = append(s.Relazioni, worker.RelazioneSTEP{Padre: p, Figlio: f, Qta: qta})
	}
	return nodi, s
}

func perChiaveNodi(p Piano) map[string]PropostaNodo {
	out := map[string]PropostaNodo{}
	for _, n := range p.Nodi {
		out[n.Chiave] = n
	}
	return out
}

func perCoppia(p Piano) map[string]PropostaRelazione {
	out := map[string]PropostaRelazione{}
	for _, r := range p.Relazioni {
		out[r.Padre+">"+r.Figlio] = r
	}
	return out
}

func TestUnNodoConFigliEPropostoSottoassiemeEUnaFogliaSciolto(t *testing.T) {
	nodi, s := file([]string{"A"}, "A>B", "B>C")
	p := perChiaveNodi(Pianifica(nodi, s, Contesto{Working: nuovaBom().working()}))
	if p["A"].Tipo != db.TipoComponenteSottoassieme || p["B"].Tipo != db.TipoComponenteSottoassieme || p["C"].Tipo != db.TipoComponenteSciolto {
		t.Errorf("tipi: A=%s B=%s C=%s", p["A"].Tipo, p["B"].Tipo, p["C"].Tipo)
	}
	for _, n := range p {
		if n.Stato != db.StatoPropostaAperta || n.ComponenteID.Valid {
			t.Errorf("in una BOM vuota ogni nodo e' una proposta aperta: %+v", n)
		}
	}
}

func TestLaRadiceCheCoincideConUnIdentificativoEPropostaFinito(t *testing.T) {
	nodi, s := file([]string{"77722757"}, "77722757>X")
	p := perChiaveNodi(Pianifica(nodi, s, Contesto{Working: nuovaBom().working(), Identificativi: map[string]bool{"77722757": true}}))
	if p["77722757"].Tipo != db.TipoComponenteFinito {
		t.Errorf("la radice che e' un identificativo della richiesta e' un prodotto finito: %s", p["77722757"].Tipo)
	}
	// un figlio con il codice di un identificativo non e' una radice: resta quello che e' nel file
	nodi, s = file([]string{"R"}, "R>77722757")
	p = perChiaveNodi(Pianifica(nodi, s, Contesto{Working: nuovaBom().working(), Identificativi: map[string]bool{"77722757": true}}))
	if p["77722757"].Tipo != db.TipoComponenteSciolto {
		t.Errorf("solo una radice si propone finito: %s", p["77722757"].Tipo)
	}
}

func TestUnCodiceGiaComponenteDiventaDuplicatoAgganciato(t *testing.T) {
	b := nuovaBom("A", "B", "Z")
	arch := b.comp["Z"]
	ora := time.Now()
	arch.ArchiviatoIl = &ora
	b.comp["Z"] = arch
	nodi, s := file([]string{"A"}, "A>B", "A>Z", "A>N")
	p := perChiaveNodi(Pianifica(nodi, s, Contesto{Working: b.working()}))
	for _, k := range []string{"A", "B"} {
		if p[k].Stato != db.StatoPropostaDuplicato || p[k].ComponenteID.UUID != b.comp[k].ComponenteID {
			t.Errorf("%s e' gia' nella BOM: atteso duplicato agganciato, trovato %+v", k, p[k])
		}
	}
	if p["N"].Stato != db.StatoPropostaAperta {
		t.Errorf("N e' nuovo: %+v", p["N"])
	}
	if p["Z"].Stato != db.StatoPropostaAperta || !strings.Contains(p["Z"].Nota, "archiviato") {
		t.Errorf("Z e' archiviato: resta aperta, e accettarla lo ripristina: %+v", p["Z"])
	}
}

// Prova 9 (L1): la differenza dello STEP vede aggiunte, rimozioni, quantita' e cambi di padre.
//
//	working  A → B ×1, A → C ×1, A → D ×1
//	file     A → B ×2, A → C, C → D, A → F        (letto per intero)
func TestLaDifferenzaDelloStepVedeAggiunteRimozioniQtaEPadri(t *testing.T) {
	b := nuovaBom("A", "B", "C", "D").arco("A", "B", 1).arco("A", "C", 1).arco("A", "D", 1)
	nodi, s := file([]string{"A"}, "A>B*2", "A>C", "C>D", "A>F")
	piano := Pianifica(nodi, s, Contesto{Working: b.working(), Completa: true})
	r := perCoppia(piano)
	if x := r["A>B"]; x.Stato != db.StatoPropostaAperta || x.Nota != "qta diversa: 2 contro 1" || x.Evidenza["qta_working"] != int32(1) {
		t.Errorf("quantita' diversa: %+v", x)
	}
	if x := r["A>C"]; x.Stato != db.StatoPropostaDuplicato {
		t.Errorf("arco uguale: %+v", x)
	}
	if x := r["C>D"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("nuovo padre di D: %+v", x)
	}
	if x := r["A>F"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("arco verso un nodo nuovo: %+v", x)
	}
	if n := perChiaveNodi(piano)["F"]; n.Stato != db.StatoPropostaAperta {
		t.Errorf("F e' un nodo nuovo: %+v", n)
	}
	// il lato «vecchio padre» e' una rimozione, e la calcola il confronto con lo STEP strutturale
	nelFile := map[Arco]bool{}
	id := func(c string) uuid.UUID { return b.comp[c].ComponenteID }
	for _, x := range [][2]string{{"A", "B"}, {"A", "C"}, {"C", "D"}} {
		nelFile[Arco{Padre: id(x[0]), Figlio: id(x[1])}] = true
	}
	rim := Rimozioni(id("A"), b.working().Archi, nelFile)
	if len(rim) != 1 || rim[0].Padre != id("A") || rim[0].Figlio != id("D") || rim[0].QtaWorking != 1 {
		t.Errorf("rimozioni = %+v, attesa solo A → D", rim)
	}
}

// Prova 33 (L1): una lettura troncata non propone quantita' (e le rimozioni non le chiede nemmeno:
// il confronto con lo STEP strutturale parte solo da una lettura completa, vedi AggiornaRimozioni).
func TestUnoStepTroncatoNonProponeQuantita(t *testing.T) {
	b := nuovaBom("A", "B").arco("A", "B", 1)
	nodi, s := file([]string{"A"}, "A>B*2", "A>N")
	r := perCoppia(Pianifica(nodi, s, Contesto{Working: b.working(), Completa: false}))
	if x := r["A>B"]; x.Stato != db.StatoPropostaDuplicato || !strings.Contains(x.Nota, "lettura incompleta") {
		t.Errorf("una lettura parziale puo' aver contato meno occorrenze: %+v", x)
	}
	// le aggiunte si propongono lo stesso (prova 34)
	if x := r["A>N"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("l'aggiunta di una lettura parziale e' una proposta: %+v", x)
	}
}

// La radice dello STEP strutturale e' il prodotto per dichiarazione, anche se il file la chiama in un
// altro modo; la schermata lo dice (A4.4).
func TestLaRadiceDelloStepStrutturaleEIlProdottoAncheSeIlNomeDiceAltro(t *testing.T) {
	b := nuovaBom("X1", "B").arco("X1", "B", 1)
	x := b.comp["X1"]
	nodi, s := file([]string{"ALTRO"}, "ALTRO>B")
	p := Pianifica(nodi, s, Contesto{Working: b.working(), Radice: &x, Completa: true})
	n := perChiaveNodi(p)["ALTRO"]
	if n.Stato != db.StatoPropostaDuplicato || n.ComponenteID.UUID != x.ComponenteID || n.Tipo != db.TipoComponenteFinito {
		t.Fatalf("la radice dichiarata: %+v", n)
	}
	if !strings.Contains(n.Nota, "il file la chiama ALTRO") {
		t.Errorf("nota: %q", n.Nota)
	}
	if r := perCoppia(p)["ALTRO>B"]; r.Stato != db.StatoPropostaDuplicato {
		t.Errorf("l'arco X1 → B c'e' gia': %+v", r)
	}
}

// Due PRODUCT con lo stesso codice nello stesso file (configurazioni) e un arco fra loro: sarebbe un
// pezzo dentro se stesso. Non si propone.
func TestUnArcoFraDueNodiDelloStessoComponenteNonSiPropone(t *testing.T) {
	b := nuovaBom("A")
	nodi := []NodoClassificato{{Chiave: "#1", Codice: "A"}, {Chiave: "#2", Codice: "a"}}
	s := worker.StrutturaSTEP{Versione: 3, Radici: []string{"#1"}, Nodi: []worker.NodoSTEP{{Chiave: "#1"}, {Chiave: "#2"}},
		Relazioni: []worker.RelazioneSTEP{{Padre: "#1", Figlio: "#2", Qta: 1}}}
	r := Pianifica(nodi, s, Contesto{Working: b.working()}).Relazioni[0]
	if r.Stato != db.StatoPropostaScartata {
		t.Errorf("A dentro A: %+v", r)
	}
}

func TestUnCicloVieneRifiutato(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	archi := []Arco{{a, b}, {b, c}, {a, d}}
	if giro := CreerebbeCiclo(archi, b, a); len(giro) != 2 || giro[0] != a || giro[1] != b {
		t.Errorf("diretto: B → A con A → B: %v", giro)
	}
	if giro := CreerebbeCiclo(archi, c, a); len(giro) != 3 {
		t.Errorf("indiretto: C → A con A → B → C: %v", giro)
	}
	if giro := CreerebbeCiclo(archi, d, c); giro != nil {
		t.Errorf("un secondo padre non e' un ciclo: %v", giro)
	}
	if giro := CreerebbeCiclo(archi, a, a); giro == nil {
		t.Error("un pezzo dentro se stesso e' un ciclo")
	}
}

func TestLaRadiceDiFamigliaAggiornaLaPropostaDelDocumento(t *testing.T) {
	m := motoreConFamiglia(t)
	s := worker.StrutturaSTEP{Versione: 3, Radici: []string{"#1"}, Nodi: []worker.NodoSTEP{nodo("#1", "", "77722757_B", "", ""), nodo("#2", "", "1234567A", "", "")}}
	r, ok := RadiceDiFamiglia(ClassificaNodi(m, s), s)
	if !ok || r.Codice != "77722757" || r.Rev != "B" {
		t.Fatalf("radice di famiglia: %+v %v", r, ok)
	}
	// un generico non basta
	s.Radici = []string{"#2"}
	if _, ok := RadiceDiFamiglia(ClassificaNodi(m, s), s); ok {
		t.Error("un generico sulla radice non corregge la proposta del documento")
	}
	// e con due radici non c'e' «la» radice
	s.Radici = []string{"#1", "#2"}
	if _, ok := RadiceDiFamiglia(ClassificaNodi(m, s), s); ok {
		t.Error("con due radici non si sceglie")
	}
}
