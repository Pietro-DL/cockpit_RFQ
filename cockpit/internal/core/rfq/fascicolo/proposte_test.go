package fascicolo

// L1 — B8.5: la classificazione dei nodi (A1.2, D16), il confronto con la working (A1.1, A4.4; Smistamento F5:
// solo nell'autorita' di un file autorizzato, A5.4.4), i cicli e le rimozioni come regole pure. Le prove L4 degli stessi casi, con le tabelle, stanno in
// proposte_db_test.go.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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

/// ---------------------------------------------------------------- il confronto con la working

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
	return NuovaWorking(b.rel)
}

// id e' il componente di un codice della BOM di prova.
func (b *bomProva) id(c string) uuid.UUID { return b.comp[c].ComponenteID }

// decisi: i nodi del file (che si chiamano come il loro codice) decisi da una persona come i componenti
// con lo stesso nome nella BOM di prova.
func (b *bomProva) decisi(nodi ...string) map[string]uuid.UUID {
	out := map[string]uuid.UUID{}
	for _, n := range nodi {
		out[n] = b.id(n)
	}
	return out
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

// nessunComponente dice se nessuna riga di nodo che il piano scriverebbe porta un componente: e' la
// colonna che una lettura non scrive mai (F5).
func nessunComponente(t *testing.T, p Piano) {
	t.Helper()
	for _, n := range p.Nodi {
		if a := argNodo(uuid.New(), uuid.New(), strings.Repeat("0", 64), n); a.ComponenteID.Valid || a.Stato != db.StatoPropostaAperta {
			t.Errorf("il nodo %s porterebbe un componente o uno stato deciso: %+v", n.Chiave, a)
		}
	}
}

func TestUnNodoConFigliEPropostoSottoassiemeEUnaFogliaSciolto(t *testing.T) {
	nodi, s := file([]string{"A"}, "A>B", "B>C")
	piano := Pianifica(nodi, s, Contesto{Working: nuovaBom().working()})
	p := perChiaveNodi(piano)
	if p["A"].Tipo != db.TipoComponenteSottoassieme || p["B"].Tipo != db.TipoComponenteSottoassieme || p["C"].Tipo != db.TipoComponenteSciolto {
		t.Errorf("tipi: A=%s B=%s C=%s", p["A"].Tipo, p["B"].Tipo, p["C"].Tipo)
	}
	for _, n := range p {
		if n.Stato != db.StatoPropostaAperta {
			t.Errorf("in una BOM vuota ogni nodo e' una proposta aperta: %+v", n)
		}
	}
	nessunComponente(t, piano)
}

// Riscritta per lo Smistamento (F5, E07): prima fissava che la radice con il codice di un identificativo
// della richiesta si proponesse «finito» (e un figlio con quel codice no). Adesso il tipo della radice lo da'
// l'autorizzazione, non il codice: la radice e' un assieme se ha figli nel file, e un figlio con il codice
// della richiesta resta quello che e' nel file. Il Contesto non ha piu' gli identificativi.
func TestLaRadiceConIlCodiceDellaRichiestaNonDiventaFinito(t *testing.T) {
	nodi, s := file([]string{"77722757"}, "77722757>X")
	piano := Pianifica(nodi, s, Contesto{Working: nuovaBom().working()})
	p := perChiaveNodi(piano)
	if p["77722757"].Tipo != db.TipoComponenteSottoassieme {
		t.Errorf("la radice con il codice della richiesta si propone per quello che e' nel file (assieme): %s", p["77722757"].Tipo)
	}
	nodi, s = file([]string{"R"}, "R>77722757")
	p = perChiaveNodi(Pianifica(nodi, s, Contesto{Working: nuovaBom().working()}))
	if p["77722757"].Tipo != db.TipoComponenteSciolto {
		t.Errorf("un figlio con il codice della richiesta resta quello che e' nel file: %s", p["77722757"].Tipo)
	}
	for _, n := range p {
		if n.Tipo == db.TipoComponenteFinito {
			t.Errorf("nessuna lettura propone «finito»: %+v", n)
		}
	}
}

// Il tipo suggerito non e' mai commerciale (decisioni del 27/09 ter, make/buy: lo STEP descrive la struttura del
// CAD, non la decisione di comprare un pezzo). Pianifica suggerisce sottoassieme per un nodo con figli nel file e
// sciolto per una foglia, mai commerciale ne' finito, in ogni contesto: senza autorizzazioni, con una sorgente e
// un figlio deciso come un componente commerciale, con la working che ha gia' l'arco. E il tipo con cui nasce un
// nodo accettato senza una scelta della persona (tipoVoluto per l'editor; accettaNodo usa la stessa regola) non e'
// mai commerciale, nemmeno da una riga di prima che lo proponesse. Il tipo commerciale lo sceglie solo una persona.
func TestIlTipoSuggeritoNonEMaiCommerciale(t *testing.T) {
	b := nuovaBom("P", "K")
	k := b.comp["K"]
	k.Tipo = db.TipoComponenteCommerciale
	b.comp["K"] = k
	nodi, s := file([]string{"P", "Z"}, "P>A", "P>K", "A>B*2", "B>C", "B>D", "K>V", "Z>A")
	sott, sciolto := db.TipoComponenteSottoassieme, db.TipoComponenteSciolto
	attesi := map[string]db.TipoComponente{"P": sott, "Z": sott, "A": sott, "K": sott, "B": sott, "C": sciolto, "D": sciolto, "V": sciolto}
	contesti := []Contesto{
		{Working: b.working()},
		{Working: b.working(), Sorgenti: map[string]uuid.UUID{"P": b.id("P")}, Decisi: b.decisi("K"), Completa: true},
		{Working: b.arco("P", "K", 1).working(), Sorgenti: map[string]uuid.UUID{"P": b.id("P"), "K": b.id("K")}},
	}
	for i, cx := range contesti {
		p := perChiaveNodi(Pianifica(nodi, s, cx))
		if len(p) != len(attesi) {
			t.Fatalf("contesto %d: %d nodi, attesi %d", i, len(p), len(attesi))
		}
		for chiave, n := range p {
			if n.Tipo == db.TipoComponenteCommerciale || n.Tipo == db.TipoComponenteFinito || n.Tipo != attesi[chiave] {
				t.Errorf("contesto %d, nodo %s: tipo %s, atteso %s", i, chiave, n.Tipo, attesi[chiave])
			}
		}
	}
	for _, proposto := range []db.TipoComponente{"", sciolto, sott, db.TipoComponenteCommerciale, db.TipoComponenteFinito} {
		var p db.ComponenteProposta
		if proposto != "" {
			p.TipoProposto = db.NullTipoComponente{TipoComponente: proposto, Valid: true}
		}
		for _, figli := range []int{0, 2} {
			if got := tipoVoluto(p, figli); got == db.TipoComponenteCommerciale || got == db.TipoComponenteFinito {
				t.Errorf("tipoVoluto(proposto %q, %d figli) = %s", proposto, figli, got)
			}
		}
	}
}

// Prova 88 (Smistamento F5): fuori dall'autorita' nessun nodo si aggancia. Riscritta: prima era
// TestUnCodiceGiaComponenteDiventaDuplicatoAgganciato, e fissava che un nodo con il codice di un componente
// della working diventasse «duplicato» agganciato a quel componente, e che un archiviato restasse aperto con la
// nota «accettarla lo ripristina». Adesso, senza sorgenti, tutto e' aperto, senza componente, senza note dal
// codice; anche gli archi che la working ha gia' restano aperti (nessuno li ha decisi).
func TestFuoriDallAutoritaNessunNodoSiAggancia(t *testing.T) {
	b := nuovaBom("A", "B", "Z").arco("A", "B", 1).arco("A", "Z", 1)
	arch := b.comp["Z"]
	ora := time.Now()
	arch.ArchiviatoIl = &ora
	b.comp["Z"] = arch
	nodi, s := file([]string{"A"}, "A>B", "A>Z", "A>N")
	piano := Pianifica(nodi, s, Contesto{Working: b.working(), Completa: true})
	p := perChiaveNodi(piano)
	for _, k := range []string{"A", "B", "N", "Z"} {
		if p[k].Stato != db.StatoPropostaAperta {
			t.Errorf("%s: fuori dall'autorita' il codice uguale non aggancia: %+v", k, p[k])
		}
	}
	if strings.Contains(fmt.Sprint(p["Z"]), "archiviato") {
		t.Errorf("Z: la nota dell'archiviato era una conseguenza del codice uguale: %+v", p["Z"])
	}
	for k, r := range perCoppia(piano) {
		if r.Stato != db.StatoPropostaAperta || r.Evidenza["qta_working"] != nil {
			t.Errorf("%s: fuori dall'autorita' nessun arco tiene i conti con la working: %+v", k, r)
		}
	}
	nessunComponente(t, piano)
	// e anche con i nodi decisi da una persona: il padre non e' una sorgente, l'arco resta aperto (195)
	piano = Pianifica(nodi, s, Contesto{Working: b.working(), Decisi: b.decisi("A", "B"), Completa: true})
	if r := perCoppia(piano)["A>B"]; r.Stato != db.StatoPropostaAperta {
		t.Errorf("A → B con i nodi decisi ma senza autorizzazione: %+v", r)
	}
}

// Prova 9 (L1): la differenza dello STEP vede aggiunte, rimozioni, quantita' e cambi di padre.
//
//	working  A → B ×1, A → C ×1, A → D ×1
//	file     A → B ×2, A → C, C → D, A → F        (letto per intero)
//
// Riscritta per lo Smistamento (F5, Domanda 1 = B, A5.4.6): prima A era la radice dello STEP strutturale e i nodi B, C, D
// erano gli stessi componenti per codice; la differenza vedeva tutto l'albero (anche C → D) e le rimozioni
// scendevano il sottoalbero. Adesso il file e' autorizzato per A (la sorgente) e B, C, D sono decisi da una
// persona: l'autorita' e' sugli archi A → figli diretti, quindi A → B e' la quantita' diversa, A → C e' uguale,
// A → F e' un'aggiunta; C → D e' GUIDA (C non ha uno STEP autorizzato suo) e resta aperto senza conti; la
// rimozione A → D la vede il confronto a profondita' 1.
func TestLaDifferenzaDelloStepVedeAggiunteRimozioniQtaEPadri(t *testing.T) {
	b := nuovaBom("A", "B", "C", "D").arco("A", "B", 1).arco("A", "C", 1).arco("A", "D", 1)
	nodi, s := file([]string{"A"}, "A>B*2", "A>C", "C>D", "A>F")
	piano := Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"A": b.id("A")},
		Decisi: b.decisi("B", "C", "D"), Completa: true})
	r := perCoppia(piano)
	if x := r["A>B"]; x.Stato != db.StatoPropostaAperta || x.Nota != "qta diversa: 2 contro 1" || x.Evidenza["qta_working"] != int32(1) {
		t.Errorf("quantita' diversa: %+v", x)
	}
	if x := r["A>C"]; x.Stato != db.StatoPropostaDuplicato {
		t.Errorf("arco uguale: %+v", x)
	}
	if x := r["C>D"]; x.Stato != db.StatoPropostaAperta || x.Evidenza["qta_working"] != nil {
		t.Errorf("C → D e' guida: aperto, senza conti con la working: %+v", x)
	}
	if x := r["A>F"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("arco verso un nodo nuovo: %+v", x)
	}
	if n := perChiaveNodi(piano)["F"]; n.Stato != db.StatoPropostaAperta {
		t.Errorf("F e' un nodo nuovo: %+v", n)
	}
	nessunComponente(t, piano)
	// il lato «vecchio padre» e' una rimozione, a profondita' 1: il file autorizzato per A contiene A → B e A → C
	nelFile := map[Arco]bool{{Padre: b.id("A"), Figlio: b.id("B")}: true, {Padre: b.id("A"), Figlio: b.id("C")}: true}
	rim := RimozioniFigliDiretti(b.id("A"), b.working().Archi, nelFile)
	if len(rim) != 1 || rim[0].Padre != b.id("A") || rim[0].Figlio != b.id("D") || rim[0].QtaWorking != 1 {
		t.Errorf("rimozioni = %+v, attesa solo A → D", rim)
	}
}

// Prova 33 (L1): una lettura troncata non propone quantita' (e le rimozioni non le chiede nemmeno:
// il confronto con il file autorizzato parte solo da una lettura completa, vedi AggiornaRimozioni).
//
// Riscritta per lo Smistamento (F5): prima A e B erano gli stessi componenti per codice; adesso A e' la
// sorgente del file autorizzato e B e' deciso da una persona. Il resto e' com'era.
func TestUnoStepTroncatoNonProponeQuantita(t *testing.T) {
	b := nuovaBom("A", "B").arco("A", "B", 1)
	nodi, s := file([]string{"A"}, "A>B*2", "A>N")
	r := perCoppia(Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"A": b.id("A")},
		Decisi: b.decisi("B"), Completa: false}))
	if x := r["A>B"]; x.Stato != db.StatoPropostaDuplicato || !strings.Contains(x.Nota, "lettura incompleta") {
		t.Errorf("una lettura parziale puo' aver contato meno occorrenze: %+v", x)
	}
	// le aggiunte si propongono lo stesso (prova 34)
	if x := r["A>N"]; x.Stato != db.StatoPropostaAperta {
		t.Errorf("l'aggiunta di una lettura parziale e' una proposta: %+v", x)
	}
}

// Prova 89 (Smistamento F5, K19): nell'autorita' nessuna riga si decide da sola. Riscritta: prima era
// TestLaRadiceDelloStepStrutturaleEIlProdottoAncheSeIlNomeDiceAltro, e fissava che la radice dichiarata
// diventasse «duplicato» del prodotto, di tipo finito, con la nota «il file la chiama ALTRO», e l'arco verso
// B un duplicato. Adesso la radice e' SORGENTE solo nel contesto (Sorgenti): la riga resta aperta, senza
// componente e senza nota (la nota si calcola in lettura: ComeSorgente); l'arco ALTRO → B, con B deciso da una
// persona e l'arco uguale nella working, e' la tenuta dei conti (duplicato); un figlio con il codice di un
// componente, non deciso, resta aperto: il codice uguale e' un suggerimento, non un'identita'.
func TestNellAutoritaNessunaRigaSiDecideDaSola(t *testing.T) {
	b := nuovaBom("X1", "B", "Y1").arco("X1", "B", 1)
	x := b.comp["X1"]
	nodi, s := file([]string{"ALTRO"}, "ALTRO>B", "ALTRO>Y1")
	piano := Pianifica(nodi, s, Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"ALTRO": x.ComponenteID},
		Decisi: b.decisi("B"), Completa: true})
	n := perChiaveNodi(piano)["ALTRO"]
	if n.Stato != db.StatoPropostaAperta || n.Tipo == db.TipoComponenteFinito {
		t.Fatalf("la sorgente resta aperta e non diventa «finito» da una lettura: %+v", n)
	}
	if y := perChiaveNodi(piano)["Y1"]; y.Stato != db.StatoPropostaAperta {
		t.Errorf("Y1 ha il codice di un componente ma nessuno l'ha deciso: resta aperto, %+v", y)
	}
	if r := perCoppia(piano)["ALTRO>B"]; r.Stato != db.StatoPropostaDuplicato {
		t.Errorf("l'arco X1 → B c'e' gia' e B e' deciso: tenuta dei conti, %+v", r)
	}
	if r := perCoppia(piano)["ALTRO>Y1"]; r.Stato != db.StatoPropostaAperta {
		t.Errorf("l'arco verso Y1, non deciso, resta aperto: %+v", r)
	}
	nessunComponente(t, piano)
	// la nota e il componente si vedono in lettura, senza scriverli
	riga := db.ComponenteProposta{Chiave: "ALTRO", Codice: pgtype.Text{String: "ALTRO", Valid: true}, Stato: db.StatoPropostaAperta}
	vista := ComeSorgente(riga, Dichiarazione{Componente: x})
	if vista.ComponenteID.UUID != x.ComponenteID || !strings.Contains(vista.Nota.String, "radice dello STEP autorizzato di X1: il file la chiama ALTRO") {
		t.Errorf("la sorgente in lettura: %+v", vista)
	}
	if riga.Stato != db.StatoPropostaAperta || riga.ComponenteID.Valid {
		t.Errorf("la lettura non cambia la riga: %+v", riga)
	}
}

// Due PRODUCT con lo stesso codice nello stesso file (configurazioni) e un arco fra loro: sarebbe un
// pezzo dentro se stesso. Non si propone.
//
// Riscritta per lo Smistamento (F5): prima i due nodi erano A per il codice; adesso #1 e' la sorgente del
// file autorizzato per A e #2 e' deciso da una persona come A. Senza l'autorizzazione l'arco resta aperto.
func TestUnArcoFraDueNodiDelloStessoComponenteNonSiPropone(t *testing.T) {
	b := nuovaBom("A")
	nodi := []NodoClassificato{{Chiave: "#1", Codice: "A"}, {Chiave: "#2", Codice: "a"}}
	s := worker.StrutturaSTEP{Versione: 3, Radici: []string{"#1"}, Nodi: []worker.NodoSTEP{{Chiave: "#1"}, {Chiave: "#2"}},
		Relazioni: []worker.RelazioneSTEP{{Padre: "#1", Figlio: "#2", Qta: 1}}}
	c := Contesto{Working: b.working(), Sorgenti: map[string]uuid.UUID{"#1": b.id("A")}, Decisi: map[string]uuid.UUID{"#2": b.id("A")}}
	r := Pianifica(nodi, s, c).Relazioni[0]
	if r.Stato != db.StatoPropostaScartata {
		t.Errorf("A dentro A: %+v", r)
	}
	if r := Pianifica(nodi, s, Contesto{Working: b.working()}).Relazioni[0]; r.Stato != db.StatoPropostaAperta {
		t.Errorf("senza autorizzazione il codice uguale non chiude l'arco: %+v", r)
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

// Riscritta per lo Smistamento (F4, D16 → evidenza, D49): prima era TestLaRadiceDiFamigliaAggiornaLaPropostaDelDocumento
// e fissava RadiceDiFamiglia, che con una radice di famiglia correggeva la proposta del documento e con un
// generico non faceva niente. Adesso la radice e' un'EVIDENZA del codice del documento, di famiglia o generica,
// con la famiglia, dove sta nel PRODUCT e il testo; la rev del file (rev_grezza) si distingue da quella che la
// famiglia separa. Con due radici, o con un codice scritto dall'operatore sul nodo, non c'e' una radice.
func TestLaRadiceDelloStepEUnEvidenzaDelDocumento(t *testing.T) {
	m := motoreConFamiglia(t)
	s := worker.StrutturaSTEP{Versione: 3, Radici: []string{"#1"}, Nodi: []worker.NodoSTEP{nodo("#1", "", "77722757_B", "", ""), nodo("#2", "", "1234567A", "", "")}}
	r, ok := RadiceDelloStep(ClassificaNodi(m, s), s)
	if !ok || r.Codice != "77722757" || r.Rev != "B" || !r.DiFamiglia || r.Famiglia != "disegni 777" || r.Dove != DoveNome ||
		r.Testo != "77722757_B" || r.RevDalFile {
		t.Fatalf("radice di famiglia: %+v %v", r, ok)
	}
	// la rev scritta nel file (PRODUCT_DEFINITION_FORMATION) e' del file, non della famiglia
	s.Nodi[0].RevGrezza = "C"
	if r, _ := RadiceDelloStep(ClassificaNodi(m, s), s); r.Rev != "C" || !r.RevDalFile {
		t.Errorf("la rev del file: %+v", r)
	}
	// un generico e' una radice anche lui: un'evidenza piu' debole, non niente
	s.Radici = []string{"#2"}
	r, ok = RadiceDelloStep(ClassificaNodi(m, s), s)
	if !ok || r.Codice != "1234567A" || r.DiFamiglia || r.Famiglia != "" {
		t.Errorf("la radice generica: %+v %v", r, ok)
	}
	// e con due radici non c'e' «la» radice
	s.Radici = []string{"#1", "#2"}
	if _, ok := RadiceDelloStep(ClassificaNodi(m, s), s); ok {
		t.Error("con due radici non si sceglie")
	}
	// un codice scritto dall'operatore sul nodo e' una decisione sul nodo, non una lettura del file
	s.Radici = []string{"#1"}
	nodi := ClassificaNodi(m, s)
	nodi[0].Origine = "operatore"
	if _, ok := RadiceDelloStep(nodi, s); ok {
		t.Error("il codice dell'operatore sul nodo non e' una lettura del file")
	}
	// una radice senza codice non dice niente
	s.Nodi[0] = nodo("#1", "", "ASSIEME", "", "")
	if _, ok := RadiceDelloStep(ClassificaNodi(m, s), s); ok {
		t.Error("una radice senza codice non e' un'evidenza")
	}
}
