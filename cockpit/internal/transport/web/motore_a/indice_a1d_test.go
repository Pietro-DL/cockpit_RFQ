package motore_a

// L1 — giro 5, A1d-Q2: le correzioni dell'indice fatte nello spostamento dalla PR #18 (indice.go), sulla scena ACME
// dell'indice (indice_test.go):
//   - D-CHIUS-4: lo stesso Rif ha Tipo e OrigineTipo diversi nella BOM di lavoro e in una candidata; NodoIn li distingue,
//     Nodo dà il primo (la BOM);
//   - F-15: ComponenteDelNodo, la sola ricerca del componente di un nodo (la radice della BOM di lavoro prende il
//     componente del target, la stessa radice in una candidata no);
//   - R9: Figlio.Origine distingue gli archi del file dai confermati senza STEP che entrano nel gruppo bom;
//   - R8: NelFile confronta il componente dei padri del file, anche quando lo stesso componente è deciso su due Rif;
//   - R7: CandidatiFuori esclude lo STEP della BOM di lavoro anche senza una candidata con lo stesso contenuto;
//   - F-16: Marcatura e StepProdotto così come sono, copie; Autorizzazioni nil senza le dichiarazioni. La validità con
//     le dichiarazioni vere la provano le L4 (servizio_db_test.go), perché fascicolo.Dichiarazioni si costruisce solo
//     da ValutaDichiarazioni sulle righe del database;
//   - IX-01, IX-02 della revisione di A1d-P2: una candidata con gli stessi Rif della BOM di lavoro (lo stesso STEP in
//     due allegati) non prende né la quantità confermata della BOM (QuantitaConfermata nel gruppo) né i suoi Rif per i
//     padri confermati (rifNelGruppo: in una candidata il padre che il gruppo non rappresenta è «componente:<uuid>»).
//
// I clienti di questi test sono inventati: ACME, codici 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
)

func TestIndiceIlTipoDipendeDalGruppo(t *testing.T) {
	f, e, c := scenaIndiceACME()
	// SC3 (allegato 32) ha i nodi della BOM: la sua radice, senza decisione, ha il tipo della regola (D4, D-CHIUS-4)
	sc3 := &e.Thread[0].ProdottiValutati[0].StruttureCandidate[2]
	sc3.Nodi[0].Tipo, sc3.Nodi[0].OrigineTipo = "sottoassieme", valutazione.OrigineTipoPropostoDallaRegola
	x := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	rif := maNodo(1, "#1")
	bom, cand := x.NodoIn(maP1, GruppoBOM, rif), x.NodoIn(maP1, maChiaveCand(32, rif), rif)
	if bom == nil || cand == nil || bom == cand {
		t.Fatalf("NodoIn: bom %p, candidata %p", bom, cand)
	}
	if bom.Tipo != "finito" || bom.OrigineTipo != valutazione.OrigineTipoConfermato ||
		cand.Tipo != "sottoassieme" || cand.OrigineTipo != valutazione.OrigineTipoPropostoDallaRegola {
		t.Errorf("tipi: bom %s/%s, candidata %s/%s", bom.Tipo, bom.OrigineTipo, cand.Tipo, cand.OrigineTipo)
	}
	if x.Nodo(maP1, rif) != bom {
		t.Error("Nodo non dà il primo, quello della BOM di lavoro")
	}
}

func TestIndiceComponenteDelNodo(t *testing.T) {
	x, _, _ := maIndiceACME()
	for _, c := range []struct {
		nome        string
		gruppo, rif string
		atteso      int // 0 = nil
	}{
		{"la radice della BOM di lavoro, senza decisione: il componente del target", GruppoBOM, maNodo(1, "#1"), 10},
		{"la stessa radice in una candidata: niente", maChiaveCand(32, maNodo(1, "#1")), maNodo(1, "#1"), 0},
		{"un nodo deciso", GruppoBOM, maNodo(1, "#2"), 11},
		{"un nodo senza decisione e non radice", GruppoBOM, maNodo(1, "#3"), 0},
		{"un confermato senza STEP", GruppoSenzaSTEP, maComp(12), 12},
		{"un confermato senza STEP nel gruppo bom", GruppoBOM, maComp(14), 14},
		{"un nodo che non c'è", GruppoBOM, maNodo(9, "#9"), 0},
		{"un gruppo che non c'è", "boh", maNodo(1, "#1"), 0},
	} {
		got := x.ComponenteDelNodo(maP1, c.gruppo, c.rif)
		if (c.atteso == 0) != (got == nil) || (got != nil && *got != maUUID(c.atteso)) {
			t.Errorf("%s: %v, atteso %d", c.nome, got, c.atteso)
		}
	}
	// una copia: cambiarla non cambia l'esito
	if got := x.ComponenteDelNodo(maP1, GruppoBOM, maNodo(1, "#1")); got != nil {
		*got = maUUID(99)
		if again := x.ComponenteDelNodo(maP1, GruppoBOM, maNodo(1, "#1")); again == nil || *again != maUUID(10) {
			t.Error("ComponenteDelNodo non dà una copia")
		}
	}
	if x.ComponenteDelNodo("identificativo:7120003", GruppoBOM, maNodo(1, "#1")) != nil {
		t.Error("un prodotto che non c'è")
	}
}

func TestIndiceFigliConLOrigine(t *testing.T) {
	x, _, _ := maIndiceACME()
	for _, c := range []struct {
		padre  string
		attesi map[string]string // rif del figlio → origine
	}{
		{maNodo(1, "#1"), map[string]string{maNodo(1, "#2"): OrigineFile, maNodo(1, "#3"): OrigineFile}},
		{maNodo(1, "#2"), map[string]string{maNodo(1, "#3"): OrigineFile, maComp(12): OrigineConfermata}},
		{maComp(12), map[string]string{maComp(14): OrigineConfermata}},
	} {
		got := map[string]string{}
		for _, f := range x.Figli(maP1, GruppoBOM, c.padre) {
			got[f.Nodo.Nodo.Rif] = f.Origine
			if (f.Origine == OrigineFile) != (f.Quantita != nil) {
				// un arco del file porta la quantità dei fatti; un confermato no (T-A1d-20)
				t.Errorf("figlio %s di %s: origine %s, quantità %s", f.Nodo.Nodo.Rif, c.padre, f.Origine, maQ(f.Quantita))
			}
		}
		if !reflect.DeepEqual(got, c.attesi) {
			t.Errorf("Figli(bom, %s) = %v, attesi %v", c.padre, got, c.attesi)
		}
	}
}

// R8: il componente 10 deciso anche su un secondo Rif del gruppo (#3 del file, sotto #1 e #2): il legame confermato
// 10 → 11 è nel file perché un padre del file di #2 (#1) ha il componente 10, qualunque sia il primo Rif che lo
// rappresenta nel gruppo.
func TestIndiceNelFilePerComponente(t *testing.T) {
	f, e, c := scenaIndiceACME()
	p := &e.Thread[0].ProdottiValutati[0]
	// #3 deciso come 10, e messo PRIMA di #1 nel gruppo: rifNelGruppo di 10 diventa #3, che non è un padre di #2
	n3 := p.Nodi[2]
	d := *p.Nodi[1].Nodo.Decisione
	d.ComponenteID, d.Codice = maUUID(10), "7120001"
	n3.Nodo.Decisione = &d
	p.Nodi = []valutazione.NodoBOM{n3, p.Nodi[0], p.Nodi[1]}
	x := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	l := x.Legami(maP1, GruppoBOM, maNodo(1, "#2"))
	var trovato bool
	for _, cf := range l.Confermati {
		if cf.PadreID == maUUID(10) {
			trovato = true
			if cf.PadreRif != maNodo(1, "#3") || !cf.NelFile {
				t.Errorf("il legame 10 → 11: PadreRif %s, NelFile %v; attesi #3 e vero (il padre del file #1 è anche lui 10)", cf.PadreRif, cf.NelFile)
			}
		}
	}
	if !trovato {
		t.Fatalf("nessun legame confermato da 10: %+v", l.Confermati)
	}
}

// IX-01 e IX-02: SC3 (allegato 32) ha lo stesso sha256 della BOM di lavoro (31), quindi gli stessi Rif. Il suo #2 è deciso
// come 11, come nella BOM; la radice della candidata non prende il componente del target (T-B4-38). Nella candidata:
// l'arco #1 → #2 non ha una quantità confermata (nella struttura di 32 non c'è una Decisione), e il padre confermato 10
// di 11 non è un nodo del gruppo, quindi è «componente:<10>», non il Rif della radice della BOM (che nella candidata
// sarebbe la radice della candidata). Nella BOM di lavoro resta tutto come prima.
func TestIndiceUnaCandidataConGliStessiRifDellaBOM(t *testing.T) {
	f, e, c := scenaIndiceACME()
	p := &e.Thread[0].ProdottiValutati[0]
	sc3 := &p.StruttureCandidate[2]
	d := *p.Nodi[1].Nodo.Decisione
	sc3.Nodi[1].Nodo.Decisione = &d
	x := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	cand := maChiaveCand(32, maNodo(1, "#1"))

	figli := x.Figli(maP1, cand, maNodo(1, "#1"))
	if len(figli) != 2 || figli[0].Nodo != &sc3.Nodi[1] {
		t.Fatalf("Figli(SC3, #1): %d figli", len(figli))
	}
	if figli[0].QuantitaConfermata != nil || figli[0].Decisa || maQ(figli[0].Quantita) != "2" {
		t.Errorf("Figli(SC3, #1)[0]: ×%s ✓%s decisa=%v; attesi ×2, nessuna quantità confermata, non decisa (IX-01)",
			maQ(figli[0].Quantita), maQ(figli[0].QuantitaConfermata), figli[0].Decisa)
	}
	l := x.Legami(maP1, cand, maNodo(1, "#2"))
	if len(l.DelFile) != 1 || l.DelFile[0].QuantitaConfermata != nil || l.DelFile[0].Decisa {
		t.Errorf("Legami(SC3, #2).DelFile = %+v: la candidata non ha la quantità confermata della BOM (IX-01)", l.DelFile)
	}
	attesi := []LegameConfermato{
		{PadreID: maUUID(10), PadreRif: maComp(10), Codice: "7120001", Qta: 2, NelFile: false},
		{PadreID: maUUID(15), PadreRif: maComp(15), Codice: "7120099", Qta: 1, NelFile: false},
	}
	if !reflect.DeepEqual(l.Confermati, attesi) {
		t.Errorf("Legami(SC3, #2).Confermati = %+v, attesi %+v (IX-02: il padre 10 non è la radice della candidata)", l.Confermati, attesi)
	}
	for _, lc := range l.Confermati {
		if n := x.NodoIn(maP1, cand, lc.PadreRif); n != nil {
			t.Errorf("il PadreRif %s del legame confermato è un nodo della candidata", lc.PadreRif)
		}
	}
	// la BOM di lavoro non cambia
	lb := x.Legami(maP1, GruppoBOM, maNodo(1, "#2"))
	if len(lb.DelFile) != 1 || maQ(lb.DelFile[0].QuantitaConfermata) != "2" || len(lb.Confermati) != 2 ||
		lb.Confermati[0].PadreRif != maNodo(1, "#1") || !lb.Confermati[0].NelFile {
		t.Errorf("Legami(bom, #2) = %+v", lb)
	}
}

func TestIndiceCandidatiFuoriSenzaLaBOM(t *testing.T) {
	f, e, c := scenaIndiceACME()
	// senza SC3 (lo stesso contenuto della BOM in 32), lo STEP della BOM (31) resta fuori per la BOM stessa
	p := &e.Thread[0].ProdottiValutati[0]
	p.StruttureCandidate = p.StruttureCandidate[:2]
	x := nuovoIndice3(&f.Thread[0], &e.Thread[0], &c)
	var fuori []string
	for _, cand := range x.CandidatiFuori(maP1) {
		fuori = append(fuori, cand.AllegatoID.String())
	}
	if !reflect.DeepEqual(fuori, []string{maUUID(36).String()}) {
		t.Errorf("CandidatiFuori = %v, atteso solo 36 (31 è la BOM, 33 una candidata)", fuori)
	}
}

func TestIndiceMarcaturaEStepProdotto(t *testing.T) {
	f, e, c := scenaIndiceACME()
	th := &f.Thread[0]
	th.RigheComponenteProposta[0].Marcatura = &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: fascicolo.RuoloRadice,
		ComponenteID: maUUIDp(10), DocumentoID: maUUIDp(41), DichiaratoDa: maUUIDp(9), DichiaratoIl: "2026-10-01T08:00:00Z", Sospesa: true}
	th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: maUUID(10), Codice: "7120001", StepStrutturaleID: maUUIDp(41),
		NStepCorrenti: 1, AnalisiCompleta: true, Esito: "presente_analizzato"}}
	x := nuovoIndice3(th, &e.Thread[0], &c)
	m := x.Marcatura(maUUID(31), "#1")
	if m == nil || !reflect.DeepEqual(*m, *th.RigheComponenteProposta[0].Marcatura) {
		t.Fatalf("Marcatura(31, #1) = %+v", m)
	}
	*m.ComponenteID = maUUID(99)
	if *th.RigheComponenteProposta[0].Marcatura.ComponenteID != maUUID(10) {
		t.Error("Marcatura non dà una copia")
	}
	if x.Marcatura(maUUID(31), "#3") != nil || x.Marcatura(maUUID(33), "#1") != nil {
		t.Error("Marcatura di una riga senza marcatura")
	}
	r := x.StepProdotto(maUUID(10))
	if r == nil || r.Esito != "presente_analizzato" || r.StepStrutturaleID == nil || *r.StepStrutturaleID != maUUID(41) {
		t.Fatalf("StepProdotto(10) = %+v", r)
	}
	*r.StepStrutturaleID = maUUID(99)
	if *th.StepProdotto[0].StepStrutturaleID != maUUID(41) {
		t.Error("StepProdotto non dà una copia")
	}
	if x.StepProdotto(maUUID(20)) != nil {
		t.Error("StepProdotto di un componente senza riga")
	}
	// senza le dichiarazioni, o con le dichiarazioni vuote, nessuna autorizzazione
	if x.Autorizzazioni(maUUID(10)) != nil {
		t.Error("Autorizzazioni senza le dichiarazioni")
	}
	vuote := fascicolo.ValutaDichiarazioni(nil)
	if a := NuovoIndice(th, &e.Thread[0], &c, &vuote).Autorizzazioni(maUUID(10)); a != nil {
		t.Errorf("Autorizzazioni con le dichiarazioni vuote: %+v", a)
	}
}
