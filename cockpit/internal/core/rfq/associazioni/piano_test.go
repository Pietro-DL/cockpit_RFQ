package associazioni

// L1 — il Piano del servizio unico (EM App. H.7; direttiva 39 §4, §5, §6): gli esiti di ogni PB, di ogni svincolo e delle
// AR dei registrati che escono, sopra fatti costruiti a mano. Dati sintetici.

import (
	"testing"

	"github.com/google/uuid"
)

func idPiano(n byte) uuid.UUID {
	return uuid.UUID{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0xab, n}
}

// fattiPiano: un'occorrenza attiva della RFQ con il codice letto ACM-1100 rev A, il componente C1 del prodotto e C9 di un
// altro prodotto.
func fattiPiano() (FattiPiano, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	a, c1, c9, p2 := idPiano(1), idPiano(11), idPiano(19), idPiano(29)
	f := FattiPiano{
		Occorrenze: map[uuid.UUID]OccorrenzaPiano{a: {DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: "aa", NomeFile: "ACM-1100_A.pdf",
			ConProposta: true, TipoProposto: "disegno_2d", CodiceLetto: "ACM-1100", RevLetta: "A"}},
		Attive: map[uuid.UUID]Associazione{}, Chiuse: map[[2]uuid.UUID]bool{}, CopieSenzaAR: map[uuid.UUID]uuid.UUID{},
		Documenti: map[uuid.UUID][]DocumentoPiano{}, DelProdotto: map[uuid.UUID]bool{c1: true}, ProdottiDi: map[uuid.UUID][]uuid.UUID{c9: {p2}},
	}
	return f, a, c1, c9, p2
}

func unaPB(f FattiPiano, v VocePB) EsitoPB {
	return Piano([]VocePB{v}, nil, nil, f).Preassociazioni[0]
}

func TestPianoEsitiDellePB(t *testing.T) {
	f, a, c1, c9, p2 := fattiPiano()
	base := VocePB{Allegato: a, ChiaveBozza: "k-telaio", Provenienza: ProvenienzaManuale, Codice: "ACM-1100", Rev: "A"}
	conC1 := base
	conC1.Componente = &c1

	if e := unaPB(f, base); e.Esito != EsitoApplicabile || e.Blocca() || !e.Applica() {
		t.Fatalf("applicabile su un nodo da creare: %+v", e)
	}
	if e := unaPB(f, VocePB{Allegato: a, ChiaveBozza: "k", Codice: "acm-1100 ", Rev: "a"}); e.Esito != EsitoApplicabile {
		t.Fatalf("il codice letto si confronta senza maiuscole ne' spazi: %+v", e)
	}
	nr := base
	nr.NonRichiesta = true
	if e := unaPB(f, nr); e.Esito != EsitoNonRichiesta || e.Blocca() || e.Applica() {
		t.Fatalf("non richiesta: %+v", e)
	}
	leg := base
	leg.Provenienza = ProvenienzaLegacy
	if e := unaPB(f, leg); e.Esito != EsitoInConflitto || e.Motivo != MotivoLegacyDaVerificare {
		t.Fatalf("legacy da verificare: %+v", e)
	}
	senzaCodice := base
	senzaCodice.Codice = ""
	if e := unaPB(f, senzaCodice); e.Esito != EsitoInAttesaCodice || !e.Blocca() {
		t.Fatalf("in attesa del codice: %+v", e)
	}
	diverso := base
	diverso.Codice = "ACM-1200"
	if e := unaPB(f, diverso); e.Motivo != MotivoCodiceDiverso {
		t.Fatalf("codice diverso: %+v", e)
	}
	revDiversa := base
	revDiversa.Rev = "B"
	if e := unaPB(f, revDiversa); e.Motivo != MotivoCodiceDiverso {
		t.Fatalf("revisione diversa: %+v", e)
	}
	senzaRev := base
	senzaRev.Rev = ""
	if e := unaPB(f, senzaRev); e.Esito != EsitoApplicabile {
		t.Fatalf("il nodo senza revisione non e' in conflitto con la revisione letta: %+v", e)
	}

	// le occorrenze
	for nome, cambia := range map[string]func(*OccorrenzaPiano){
		MotivoFuoriRFQ:                func(o *OccorrenzaPiano) { o.DellaRfq = false },
		MotivoNaturaNonPreassociabile: func(o *OccorrenzaPiano) { o.Preassociabile = false },
		MotivoLegatoComeSorgente:      func(o *OccorrenzaPiano) { o.LegatoComeSorgente = true },
		MotivoFileNonAttivo:           func(o *OccorrenzaPiano) { o.Attivo = false },
		MotivoCodiceNonLetto:          func(o *OccorrenzaPiano) { o.TipoProposto = TipoDaDeterminare },
	} {
		g, _, _, _, _ := fattiPiano()
		o := g.Occorrenze[a]
		cambia(&o)
		g.Occorrenze[a] = o
		if e := unaPB(g, base); e.Esito != EsitoInConflitto || e.Motivo != nome {
			t.Fatalf("%s: %+v", nome, e)
		}
	}
	g, _, _, _, _ := fattiPiano()
	o := g.Occorrenze[a]
	o.ConProposta, o.CodiceLetto = false, ""
	g.Occorrenze[a] = o
	if e := unaPB(g, base); e.Motivo != MotivoCodiceNonLetto {
		t.Fatalf("senza proposta di documento (Prova 2: si preassocia, non si conferma, D-1 A): %+v", e)
	}

	// le AR (Prova 7): la stessa coppia, un altro componente dello stesso prodotto (con e senza svincolo), di un altro prodotto
	g, _, _, _, _ = fattiPiano()
	g.Attive[a] = Associazione{ID: idPiano(50), Allegato: a, Componente: c1, Stato: StatoAttiva}
	if e := unaPB(g, conC1); e.Esito != EsitoGiaApplicata || e.Applica() || e.Associazione == nil {
		t.Fatalf("gia' applicata: %+v", e)
	}
	altro := idPiano(12)
	g.DelProdotto[altro] = true
	conAltro := base
	conAltro.Componente = &altro
	if e := unaPB(g, conAltro); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteStessoProdotto {
		t.Fatalf("gia' associata nello stesso prodotto: %+v", e)
	}
	conAltro.Svincolata = true
	if e := unaPB(g, conAltro); e.Esito != EsitoSostituzione || !e.Applica() {
		t.Fatalf("sostituzione con lo svincolo nella stessa bozza: %+v", e)
	}
	g.Attive[a] = Associazione{ID: idPiano(51), Allegato: a, Componente: c9, Stato: StatoAttiva}
	if e := unaPB(g, conAltro); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteAltroProdotto || len(e.ARProdotti) != 1 || e.ARProdotti[0] != p2 {
		t.Fatalf("gia' associata a un altro prodotto: lo svincolo non trasferisce il file fra prodotti: %+v", e)
	}
	// la copia legacy senza AR (punto 5 di trg_fra_nasce)
	g, _, _, _, _ = fattiPiano()
	g.CopieSenzaAR[a] = c9
	if e := unaPB(g, conC1); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteCopiaLegacy || e.CopiaComponente == nil || *e.CopiaComponente != c9 {
		t.Fatalf("copia legacy: %+v", e)
	}
	g.CopieSenzaAR[a] = c1
	if e := unaPB(g, conC1); e.Esito != EsitoApplicabile {
		t.Fatalf("la copia legacy sullo stesso componente non ferma: %+v", e)
	}
	// i documenti (D-AR06, D-AR07)
	g, _, _, _, _ = fattiPiano()
	g.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60)}}
	if e := unaPB(g, conC1); e.Motivo != MotivoDocumentoSenzaComponente {
		t.Fatalf("documento senza componente: %+v", e)
	}
	g.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60), Componente: &c9}}
	if e := unaPB(g, conC1); e.Motivo != MotivoDocumentoConfermatoAltrui {
		t.Fatalf("documento di un altro componente: %+v", e)
	}
	g.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60), Componente: &c1}}
	if e := unaPB(g, conC1); e.Esito != EsitoApplicabile {
		t.Fatalf("documento dello stesso componente: %+v", e)
	}
}

// fattiDaPartner: i fatti di fattiPiano con un file dal nome «da partner» (BOM_9120001_1010_20260101_E.pdf): la lettura
// generica del nome (documento_proposta, senza grammatica) non e' il codice del pezzo.
func fattiDaPartner() (FattiPiano, uuid.UUID, uuid.UUID, uuid.UUID) {
	f, a, c1, c9, _ := fattiPiano()
	f.Occorrenze[a] = OccorrenzaPiano{DellaRfq: true, Attivo: true, Preassociabile: true, Sha256: "aa", NomeFile: "BOM_9120001_1010_20260101_E.pdf",
		ConProposta: true, TipoProposto: "disegno_2d", CodiceLetto: "BOM_9120001_1010_20260101", RevLetta: "E"}
	return f, a, c1, c9
}

// D-48-01 A [T] (preparata in attesa della risposta): la PB del motore rimasta intatta non passa per il confronto del
// codice letto (codice_diverso, codice_non_letto); in attesa di codice e tutti i controlli di prima restano; la PB
// manuale o corretta resta confrontata (D-48-02 aperta).
func TestPianoPBDelMotoreIntatta(t *testing.T) {
	f, a, c1, c9 := fattiDaPartner()
	motore := VocePB{Allegato: a, ChiaveBozza: "k-9120001", Provenienza: ProvenienzaMotore, Codice: "9120001", Rev: "A", DalMotore: true}
	if e := unaPB(f, motore); e.Esito != EsitoApplicabile || !e.Applica() || e.CodiceLetto != "BOM_9120001_1010_20260101" {
		t.Fatalf("PB del motore intatta con il codice letto diverso: %+v", e)
	}
	for nome, cambia := range map[string]func(*OccorrenzaPiano){
		"tipo da determinare":  func(o *OccorrenzaPiano) { o.TipoProposto = TipoDaDeterminare },
		"senza proposta":       func(o *OccorrenzaPiano) { o.ConProposta, o.CodiceLetto, o.RevLetta = false, "", "" },
		"codice letto vuoto":   func(o *OccorrenzaPiano) { o.CodiceLetto = "" },
		"revisione differente": func(o *OccorrenzaPiano) { o.CodiceLetto, o.RevLetta = "9120001", "Z" },
	} {
		g, _, _, _ := fattiDaPartner()
		o := g.Occorrenze[a]
		cambia(&o)
		g.Occorrenze[a] = o
		if e := unaPB(g, motore); e.Esito != EsitoApplicabile {
			t.Fatalf("PB del motore intatta, %s: %+v", nome, e)
		}
	}
	senzaCodice := motore
	senzaCodice.Codice = ""
	if e := unaPB(f, senzaCodice); e.Esito != EsitoInAttesaCodice || !e.Blocca() {
		t.Fatalf("PB del motore su un nodo senza codice: %+v", e)
	}
	nr := motore
	nr.NonRichiesta = true
	if e := unaPB(f, nr); e.Esito != EsitoNonRichiesta {
		t.Fatalf("PB del motore non richiesta: %+v", e)
	}

	// i controlli che vengono prima restano
	for nome, cambia := range map[string]func(*OccorrenzaPiano){
		MotivoFuoriRFQ:                func(o *OccorrenzaPiano) { o.DellaRfq = false },
		MotivoNaturaNonPreassociabile: func(o *OccorrenzaPiano) { o.Preassociabile = false },
		MotivoLegatoComeSorgente:      func(o *OccorrenzaPiano) { o.LegatoComeSorgente = true },
		MotivoStepCandidatoDelFinito:  func(o *OccorrenzaPiano) { o.StepCandidatoDelFinito = true },
		MotivoFileNonAttivo:           func(o *OccorrenzaPiano) { o.Attivo = false },
	} {
		g, _, _, _ := fattiDaPartner()
		o := g.Occorrenze[a]
		cambia(&o)
		g.Occorrenze[a] = o
		if e := unaPB(g, motore); e.Esito != EsitoInConflitto || e.Motivo != nome {
			t.Fatalf("PB del motore, %s: %+v", nome, e)
		}
	}
	g, _, _, _ := fattiDaPartner()
	delete(g.Occorrenze, a)
	if e := unaPB(g, motore); e.Motivo != MotivoFuoriRFQ {
		t.Fatalf("PB del motore su un file che la RFQ non ha: %+v", e)
	}
	conC1 := motore
	conC1.Componente = &c1
	altro := idPiano(12)
	conAltro := motore
	conAltro.Componente = &altro
	g, _, _, _ = fattiDaPartner()
	g.DelProdotto[altro] = true
	g.Attive[a] = Associazione{ID: idPiano(50), Allegato: a, Componente: c1, Stato: StatoAttiva}
	if e := unaPB(g, conC1); e.Esito != EsitoGiaApplicata {
		t.Fatalf("PB del motore gia' applicata: %+v", e)
	}
	if e := unaPB(g, conAltro); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteStessoProdotto {
		t.Fatalf("PB del motore gia' associata nello stesso prodotto: %+v", e)
	}
	conAltro.Svincolata = true
	if e := unaPB(g, conAltro); e.Esito != EsitoSostituzione {
		t.Fatalf("PB del motore, sostituzione: %+v", e)
	}
	g.Attive[a] = Associazione{ID: idPiano(51), Allegato: a, Componente: c9, Stato: StatoAttiva}
	if e := unaPB(g, conAltro); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteAltroProdotto {
		t.Fatalf("PB del motore gia' associata a un altro prodotto: %+v", e)
	}
	g, _, _, _ = fattiDaPartner()
	g.CopieSenzaAR[a] = c9
	if e := unaPB(g, conC1); e.Motivo != MotivoGiaAssociataAltrove || e.Variante != VarianteCopiaLegacy {
		t.Fatalf("PB del motore e copia legacy: %+v", e)
	}
	g, _, _, _ = fattiDaPartner()
	g.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60)}}
	if e := unaPB(g, conC1); e.Motivo != MotivoDocumentoSenzaComponente {
		t.Fatalf("PB del motore, documento senza componente: %+v", e)
	}
	g.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60), Componente: &c9}}
	if e := unaPB(g, conC1); e.Motivo != MotivoDocumentoConfermatoAltrui {
		t.Fatalf("PB del motore, documento di un altro componente: %+v", e)
	}

	// le PB dell'ingegnere (e una PB motore che chi chiama non dice intatta) restano confrontate
	for _, prov := range []string{ProvenienzaManuale, ProvenienzaCorretta, ProvenienzaMotore} {
		v := motore
		v.Provenienza, v.DalMotore = prov, false
		if e := unaPB(f, v); e.Esito != EsitoInConflitto || e.Motivo != MotivoCodiceDiverso {
			t.Fatalf("PB %s (non intatta) con il codice letto diverso: %+v", prov, e)
		}
		g, _, _, _ := fattiDaPartner()
		o := g.Occorrenze[a]
		o.TipoProposto = TipoDaDeterminare
		g.Occorrenze[a] = o
		if e := unaPB(g, v); e.Motivo != MotivoCodiceNonLetto {
			t.Fatalf("PB %s (non intatta) con il tipo da determinare: %+v", prov, e)
		}
	}
	// il flag su una PB manuale o corretta non salta il confronto (D-48-02 aperta)
	for _, prov := range []string{ProvenienzaManuale, ProvenienzaCorretta} {
		v := motore
		v.Provenienza = prov
		if e := unaPB(f, v); e.Esito != EsitoInConflitto || e.Motivo != MotivoCodiceDiverso {
			t.Fatalf("PB %s con DalMotore alzato: %+v", prov, e)
		}
	}
}

func TestPianoSvincoliEUscite(t *testing.T) {
	f, a, c1, c9, _ := fattiPiano()
	b := idPiano(2)
	f.Occorrenze[b] = f.Occorrenze[a]
	f.Attive[a] = Associazione{ID: idPiano(50), Allegato: a, Componente: c1, Stato: StatoAttiva}
	f.Chiuse[[2]uuid.UUID{b, c1}] = true
	es := Piano(nil, []VoceSvincolo{{Allegato: a, Componente: c1}, {Allegato: b, Componente: c1}, {Allegato: b, Componente: c9},
		{Allegato: a, Componente: c9}}, nil, f).Svincoli
	att := map[[2]uuid.UUID]string{{a, c1}: EsitoApplicabile, {b, c1}: EsitoSvincoloGiaChiuso, {b, c9}: MotivoSvincoloSenzaAssociazione,
		{a, c9}: MotivoSvincoloComponenteDiverso}
	for _, e := range es {
		got := e.Esito
		if e.Esito == EsitoInConflitto {
			got = e.Motivo
		}
		if att[[2]uuid.UUID{e.Allegato, e.Componente}] != got {
			t.Fatalf("svincolo %s/%s: %+v", e.Allegato, e.Componente, e)
		}
	}
	f.Documenti[a] = []DocumentoPiano{{Documento: idPiano(60), Componente: &c1}}
	if e := Piano(nil, []VoceSvincolo{{Allegato: a, Componente: c1}}, nil, f).Svincoli[0]; e.Motivo != MotivoSvincoloSuDocumento || !e.Blocca() {
		t.Fatalf("svincolo di un documento confermato: %+v", e)
	}
	// le uscite: escluso → «Mantieni» (avviso), tolto senza decisione → blocco, con lo svincolo → niente
	f.Documenti = map[uuid.UUID][]DocumentoPiano{}
	u := Piano(nil, nil, []Uscita{{Componente: c1, Escluso: true}}, f).Uscite
	if len(u) != 1 || u[0].Esito != EsitoMantenuta || u[0].Blocca() {
		t.Fatalf("escluso con AR: %+v", u)
	}
	u = Piano(nil, nil, []Uscita{{Componente: c1}}, f).Uscite
	if len(u) != 1 || u[0].Motivo != MotivoNodoUscenteConAR || !u[0].Blocca() {
		t.Fatalf("tolto con AR: %+v", u)
	}
	if u = Piano(nil, []VoceSvincolo{{Allegato: a, Componente: c1}}, []Uscita{{Componente: c1}}, f).Uscite; len(u) != 0 {
		t.Fatalf("tolto con lo svincolo: %+v", u)
	}
}

func TestPianoOrdineDeterministico(t *testing.T) {
	f, a, _, _, _ := fattiPiano()
	b := idPiano(0)
	f.Occorrenze[b] = f.Occorrenze[a]
	es := Piano([]VocePB{{Allegato: a, ChiaveBozza: "x", Codice: "ACM-1100"}, {Allegato: b, ChiaveBozza: "y", Codice: "ACM-1100"}}, nil, nil, f)
	if es.Preassociazioni[0].Allegato != b || es.Preassociazioni[1].Allegato != a {
		t.Fatalf("ordine per allegato: %+v", es.Preassociazioni)
	}
}
